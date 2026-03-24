/*
Copyright 2026 The opendatahub.io Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api_translation "github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/api-translation"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/provider"
	"github.com/opendatahub-io/ai-gateway-payload-processing/test/e2e/testplugins"

	"sigs.k8s.io/gateway-api-inference-extension/pkg/bbr/framework"
	runserver "sigs.k8s.io/gateway-api-inference-extension/pkg/bbr/server"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/common/observability/logging"
)

const (
	llmKatanImage     = "llm-katan:e2e"
	llmKatanPort      = 8000
	envoyImage        = "envoyproxy/envoy:distroless-v1.33.2"
	dockerNetworkName = "e2e-azure-net"
	llmKatanContainer = "llm-katan-e2e"
)

var logger = logutil.NewTestLogger().V(logutil.VERBOSE)

type envoyTemplateData struct {
	EnvoyPort   int
	AdminPort   int
	BBRHost     string
	BBRPort     int
	BackendHost string
	BackendPort int
}

// TestAzureOpenAI_E2E verifies the full request flow through the plugin chain:
//
//	HTTP client → Envoy (Docker) → ext_proc (BBR) → llm-katan (Docker)
//
// Plugin chain: provider-setter → api-key-setter → api-translation
//
// llm-katan runs locally as a Docker container in echo mode, simulating an
// Azure OpenAI endpoint. No external endpoints or K8s cluster required.
// Prerequisites: Docker + llm-katan:e2e image (built from testdata/llm-katan.Dockerfile).
func TestAzureOpenAI_E2E(t *testing.T) {
	if os.Getenv("E2E_AZURE") == "" {
		t.Skip("Skipping Azure e2e test; set E2E_AZURE=1 to run")
	}
	requireDocker(t)
	requireImage(t, llmKatanImage)

	// Clean up resources from any previous failed run.
	cleanupOrphaned(t)

	// --- Docker network (Envoy ↔ llm-katan) ---
	networkID := createDockerNetwork(t)
	defer removeDockerNetwork(t, networkID)

	// --- llm-katan (simulated Azure OpenAI backend) ---
	llmKatanID := startLLMKatan(t, networkID)
	defer stopContainer(t, llmKatanID, "llm-katan")
	waitForHTTPHealth(t, llmKatanContainer, networkID, 30*time.Second)
	t.Log("llm-katan ready")

	// --- BBR ext_proc server (in-process) ---
	bbrPort := getFreePort(t)
	ctx, cancel := context.WithCancel(context.Background())

	startBBRServer(t, ctx, cancel, bbrPort)
	waitForTCP(t, "127.0.0.1", bbrPort, 10*time.Second)
	t.Logf("BBR ext_proc server on :%d", bbrPort)

	// --- Envoy (Docker, same network as llm-katan) ---
	envoyPort := getFreePort(t)
	adminPort := getFreePort(t)

	configPath := renderEnvoyConfig(t, envoyTemplateData{
		EnvoyPort:   envoyPort,
		AdminPort:   adminPort,
		BBRHost:     "host.docker.internal",
		BBRPort:     bbrPort,
		BackendHost: llmKatanContainer,
		BackendPort: llmKatanPort,
	})
	envoyID := startEnvoyContainer(t, configPath, envoyPort, adminPort, networkID)
	defer stopContainer(t, envoyID, "envoy")

	waitForTCP(t, "127.0.0.1", envoyPort, 30*time.Second)
	t.Logf("Envoy on :%d — waiting for ext_proc connection", envoyPort)
	time.Sleep(3 * time.Second)

	t.Run("azure_unary", func(t *testing.T) {
		resp, body := sendChatCompletion(t, envoyPort, "gpt-4o", "Hello from e2e")
		defer func() { _ = resp.Body.Close() }()

		require.Equal(t, http.StatusOK, resp.StatusCode,
			"expected 200 from llm-katan via Azure path rewrite; body: %s", body)

		var data map[string]any
		require.NoError(t, json.Unmarshal(body, &data),
			"response must be valid JSON: %s", body)

		// Verify chat completion structure from llm-katan
		assert.Contains(t, data, "choices")
		assert.Contains(t, data, "model")
		assert.Contains(t, data, "usage")

		// Verify the echo content proves the request was received in Azure format
		choices, _ := data["choices"].([]any)
		require.NotEmpty(t, choices)
		firstChoice, _ := choices[0].(map[string]any)
		msg, _ := firstChoice["message"].(map[string]any)
		content, _ := msg["content"].(string)
		assert.Contains(t, content, "[echo]",
			"response should be from llm-katan echo mode")
	})
}

// ---------------------------------------------------------------------------
// BBR helpers
// ---------------------------------------------------------------------------

func startBBRServer(t *testing.T, ctx context.Context, cancel context.CancelFunc, port int) {
	t.Helper()

	const testAPIKey = "test-e2e-azure-key"

	providerSetter := testplugins.NewProviderSetterPlugin(provider.AzureOpenAI)
	apiKeySetter := testplugins.NewApiKeySetterPlugin(testAPIKey)
	apiTranslation := api_translation.NewAPITranslationPlugin()

	runner := runserver.NewDefaultExtProcServerRunner(port, false)
	runner.SecureServing = false
	runner.RequestPlugins = []framework.RequestProcessor{providerSetter, apiKeySetter, apiTranslation}
	runner.ResponsePlugins = []framework.ResponseProcessor{apiTranslation}

	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.AsRunnable(logger.WithName("bbr-e2e")).Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("BBR server exited with error: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Docker network helpers
// ---------------------------------------------------------------------------

func createDockerNetwork(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("docker", "network", "create", dockerNetworkName).CombinedOutput()
	require.NoError(t, err, "docker network create failed: %s", string(out))
	networkID := strings.TrimSpace(string(out))
	t.Logf("Created Docker network %s (%s)", dockerNetworkName, networkID[:12])
	return networkID
}

func removeDockerNetwork(t *testing.T, networkID string) {
	t.Helper()
	out, err := exec.Command("docker", "network", "rm", networkID).CombinedOutput()
	if err != nil {
		t.Logf("Warning: docker network rm: %v — %s", err, string(out))
	} else {
		t.Logf("Removed Docker network %s", dockerNetworkName)
	}
}

// ---------------------------------------------------------------------------
// llm-katan helpers
// ---------------------------------------------------------------------------

func startLLMKatan(t *testing.T, networkID string) string {
	t.Helper()
	args := []string{
		"run", "-d",
		"--name", llmKatanContainer,
		"--network", networkID,
		llmKatanImage,
	}
	out, err := exec.Command("docker", args...).CombinedOutput()
	require.NoError(t, err, "docker run llm-katan failed: %s", string(out))
	containerID := strings.TrimSpace(string(out))
	t.Logf("Started llm-katan container %s", containerID[:12])
	return containerID
}

// waitForHTTPHealth polls llm-katan's /health endpoint from inside the Docker
// network using a temporary container, since llm-katan has no port published
// to the host.
func waitForHTTPHealth(t *testing.T, host, networkID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	healthURL := fmt.Sprintf("http://%s:%d/health", host, llmKatanPort)

	for time.Now().Before(deadline) {
		cmd := exec.Command("docker", "run", "--rm",
			"--network", networkID,
			"python:3.12-slim",
			"python", "-c",
			fmt.Sprintf("import urllib.request; urllib.request.urlopen('%s', timeout=2)", healthURL),
		)
		if err := cmd.Run(); err == nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("llm-katan health check failed within %s", timeout)
}

// ---------------------------------------------------------------------------
// Envoy Docker helpers
// ---------------------------------------------------------------------------

func renderEnvoyConfig(t *testing.T, data envoyTemplateData) string {
	t.Helper()

	tmplPath := filepath.Join("testdata", "envoy.yaml.tmpl")
	tmpl, err := template.ParseFiles(tmplPath)
	require.NoError(t, err, "failed to parse Envoy config template")

	outPath := filepath.Join(t.TempDir(), "envoy.yaml")
	f, err := os.Create(outPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	require.NoError(t, tmpl.Execute(f, data))
	t.Logf("Envoy config written to %s", outPath)
	return outPath
}

func startEnvoyContainer(t *testing.T, configPath string, envoyPort, adminPort int, networkID string) string {
	t.Helper()

	absConfig, err := filepath.Abs(configPath)
	require.NoError(t, err)

	name := fmt.Sprintf("envoy-e2e-%d", envoyPort)

	args := []string{
		"run", "-d",
		"--name", name,
		"--network", networkID,
		"--add-host=host.docker.internal:host-gateway",
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", envoyPort, envoyPort),
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", adminPort, adminPort),
		"-v", fmt.Sprintf("%s:/etc/envoy/envoy.yaml:ro", absConfig),
		envoyImage,
		"-c", "/etc/envoy/envoy.yaml",
		"--log-level", "info",
	}

	cmd := exec.Command("docker", args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "docker run envoy failed: %s", string(out))

	containerID := strings.TrimSpace(string(out))
	t.Logf("Started Envoy container %s (name=%s)", containerID[:12], name)
	return containerID
}

func containerLogs(t *testing.T, containerID, label string) {
	t.Helper()
	cmd := exec.Command("docker", "logs", "--tail", "20", containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: docker logs (%s) failed: %v", label, err)
		return
	}
	t.Logf("--- %s logs (last 20 lines) ---\n%s", label, string(out))
}

func stopContainer(t *testing.T, containerID, label string) {
	t.Helper()
	containerLogs(t, containerID, label)
	exec.Command("docker", "stop", containerID).Run()     //nolint:errcheck
	exec.Command("docker", "rm", "-f", containerID).Run() //nolint:errcheck
	t.Logf("Stopped %s container %s", label, containerID[:12])
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func sendChatCompletion(t *testing.T, envoyPort int, model, message string) (*http.Response, []byte) {
	t.Helper()

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": message},
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)

	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", envoyPort)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err, "HTTP request to Envoy failed")

	const maxResponseSize = 1 << 20 // 1 MiB
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	require.NoError(t, err)
	t.Logf("Response [%d]: %s", resp.StatusCode, string(respBody))
	return resp, respBody
}

// ---------------------------------------------------------------------------
// Generic utilities
// ---------------------------------------------------------------------------

func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForTCP(t *testing.T, host string, port int, timeout time.Duration) {
	t.Helper()
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("TCP %s not reachable within %s", addr, timeout)
}

func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

func requireImage(t *testing.T, image string) {
	t.Helper()
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skipf("Docker image %q not found — build it first: docker build -t %s -f test/e2e/testdata/llm-katan.Dockerfile test/e2e/testdata/",
			image, image)
	}
}

// cleanupOrphaned removes containers and networks left behind by a previous
// failed test run. Errors are logged but not fatal — the resources may not exist.
func cleanupOrphaned(t *testing.T) {
	t.Helper()

	exec.Command("docker", "rm", "-f", llmKatanContainer).Run() //nolint:errcheck

	// Envoy container names use a "envoy-e2e-" prefix with a random port suffix.
	out, err := exec.Command("docker", "ps", "-aq", "--filter", "name=envoy-e2e-").CombinedOutput()
	if err == nil {
		for _, id := range strings.Fields(strings.TrimSpace(string(out))) {
			exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck
		}
	}

	exec.Command("docker", "network", "rm", dockerNetworkName).Run() //nolint:errcheck
}

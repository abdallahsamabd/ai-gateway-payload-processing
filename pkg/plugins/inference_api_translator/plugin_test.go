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

package inference_api_translator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"sigs.k8s.io/gateway-api-inference-extension/pkg/bbr/framework"
)

func TestProcessRequest_NoProviderHeader(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	req := framework.NewInferenceRequest()
	req.Headers["content-type"] = "application/json"
	req.Body["model"] = "gpt-4o"
	req.Body["messages"] = []any{map[string]any{"role": "user", "content": "Hi"}}

	err = p.ProcessRequest(context.Background(), req)
	assert.NoError(t, err)
	assert.False(t, req.BodyMutated())
}

func TestProcessRequest_OpenAIProvider(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	req := framework.NewInferenceRequest()
	req.Headers[ProviderHeader] = "openai"
	req.Body["model"] = "gpt-4o"
	req.Body["messages"] = []any{map[string]any{"role": "user", "content": "Hi"}}

	err = p.ProcessRequest(context.Background(), req)
	assert.NoError(t, err)
	assert.False(t, req.BodyMutated())
}

func TestProcessRequest_AnthropicProvider(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	req := framework.NewInferenceRequest()
	req.Headers[ProviderHeader] = "anthropic"
	req.Headers["authorization"] = "Bearer sk-test"  // present so RemoveHeader can track it
	req.Headers["content-length"] = "123"             // present so RemoveHeader can track it
	req.Body["model"] = "claude-sonnet-4-20250514"
	req.Body["messages"] = []any{
		map[string]any{"role": "system", "content": "Be concise"},
		map[string]any{"role": "user", "content": "What is 2+2?"},
	}
	req.Body["max_tokens"] = float64(100)

	err = p.ProcessRequest(context.Background(), req)
	require.NoError(t, err)

	assert.True(t, req.BodyMutated())

	// Body should now be Anthropic format
	assert.Equal(t, "Be concise", req.Body["system"])
	assert.Equal(t, 100, req.Body["max_tokens"])

	msgs, ok := req.Body["messages"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, msgs, 1) // system message extracted
	assert.Equal(t, "user", msgs[0]["role"])

	// Headers should include Anthropic-specific values
	mutated := req.MutatedHeaders()
	assert.Equal(t, "2023-06-01", mutated["anthropic-version"])
	assert.Equal(t, "/v1/messages", mutated[":path"])
	assert.Equal(t, "application/json", mutated["content-type"])

	// Authorization and content-length should be removed
	removed := req.RemovedHeaders()
	assert.Contains(t, removed, "authorization")
	assert.Contains(t, removed, "content-length")
}

func TestProcessRequest_UnknownProvider(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	req := framework.NewInferenceRequest()
	req.Headers[ProviderHeader] = "bedrock"
	req.Body["model"] = "some-model"
	req.Body["messages"] = []any{map[string]any{"role": "user", "content": "Hi"}}

	err = p.ProcessRequest(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported provider")
	assert.Contains(t, err.Error(), "bedrock")
}

func TestProcessRequest_NilRequest(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	err = p.ProcessRequest(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-nil")
}

func TestProcessResponse_AnthropicDetected(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	resp := framework.NewInferenceResponse()
	resp.Body["id"] = "msg_123"
	resp.Body["type"] = "message"
	resp.Body["model"] = "claude-sonnet-4-20250514"
	resp.Body["content"] = []any{
		map[string]any{"type": "text", "text": "The answer is 4."},
	}
	resp.Body["stop_reason"] = "end_turn"
	resp.Body["usage"] = map[string]any{
		"input_tokens":  float64(10),
		"output_tokens": float64(5),
	}

	err = p.ProcessResponse(context.Background(), resp)
	require.NoError(t, err)

	assert.True(t, resp.BodyMutated())
	assert.Equal(t, "chat.completion", resp.Body["object"])
	assert.Equal(t, "claude-sonnet-4-20250514", resp.Body["model"])

	choices := resp.Body["choices"].([]any)
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	assert.Equal(t, "assistant", msg["role"])
	assert.Equal(t, "The answer is 4.", msg["content"])
	assert.Equal(t, "stop", choice["finish_reason"])

	usage := resp.Body["usage"].(map[string]any)
	assert.Equal(t, 10, usage["prompt_tokens"])
	assert.Equal(t, 5, usage["completion_tokens"])
	assert.Equal(t, 15, usage["total_tokens"])
}

func TestProcessResponse_OpenAIPassthrough(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	resp := framework.NewInferenceResponse()
	resp.Body["object"] = "chat.completion"
	resp.Body["choices"] = []any{
		map[string]any{"message": map[string]any{"content": "hi"}},
	}

	err = p.ProcessResponse(context.Background(), resp)
	assert.NoError(t, err)
	assert.False(t, resp.BodyMutated())
}

func TestProcessResponse_UnknownFormat(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	resp := framework.NewInferenceResponse()
	resp.Body["some_field"] = "some_value"

	err = p.ProcessResponse(context.Background(), resp)
	assert.NoError(t, err)
	assert.False(t, resp.BodyMutated())
}

func TestDetectProviderFromResponse(t *testing.T) {
	tests := []struct {
		name     string
		body     map[string]any
		expected string
	}{
		{
			name:     "anthropic message",
			body:     map[string]any{"type": "message", "content": []any{}},
			expected: "anthropic",
		},
		{
			name:     "anthropic error",
			body:     map[string]any{"type": "error", "error": map[string]any{"type": "invalid_request_error"}},
			expected: "anthropic",
		},
		{
			name:     "openai completion",
			body:     map[string]any{"object": "chat.completion"},
			expected: "openai",
		},
		{
			name:     "openai error",
			body:     map[string]any{"error": map[string]any{"message": "bad request"}},
			expected: "openai",
		},
		{
			name:     "unknown format",
			body:     map[string]any{"result": "something"},
			expected: "",
		},
		{
			name:     "empty body",
			body:     map[string]any{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectProviderFromResponse(tt.body)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFactory_Success(t *testing.T) {
	plugin, err := Factory("test-instance", nil)
	require.NoError(t, err)
	assert.Equal(t, "test-instance", plugin.TypedName().Name)
	assert.Equal(t, PluginType, plugin.TypedName().Type)
}

func TestFactory_ProvidersRegistered(t *testing.T) {
	p, err := NewInferenceAPITranslatorPlugin()
	require.NoError(t, err)

	_, hasAnthropic := p.providerIndex["anthropic"]
	_, hasOpenAI := p.providerIndex["openai"]

	assert.True(t, hasAnthropic, "anthropic provider should be registered")
	assert.True(t, hasOpenAI, "openai provider should be registered")
}

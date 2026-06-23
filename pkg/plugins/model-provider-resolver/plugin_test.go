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

package model_provider_resolver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/requesthandling"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/plugin"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/apiformat"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/auth"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/provider"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/state"
)

func TestProcessRequest_ModelResolved(t *testing.T) {
	store := newInfoStore()
	const (
		extNS       = "llm"
		extName     = "claude-sonnet"
		targetModel = "claude-sonnet-1234"
		credName    = "anthropic-key"
		endpoint    = "api.anthropic.com"
	)
	store.addOrUpdateModel(extName,
		&externalModelInfo{modelName: extName, refs: []*resolvedProviderRef{{
			provider:        provider.Anthropic,
			targetModel:     targetModel,
			apiFormat:       apiformat.Messages,
			auth:            auth.APIKey,
			endpoint:        endpoint,
			secretName:      credName,
			secretNamespace: extNS,
			config:          map[string]string{},
			weight:          1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/" + extNS + "/" + extName + "/v1/chat/completions"
	req.Body["model"] = extName

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err)

	actualModel, err := plugin.ReadCycleStateKey[string](cs, state.ModelKey)
	require.NoError(t, err)
	require.Equal(t, targetModel, actualModel)

	actualProvider, err := plugin.ReadCycleStateKey[string](cs, state.ProviderKey)
	require.NoError(t, err)
	require.Equal(t, provider.Anthropic, actualProvider)

	actualCredsName, err := plugin.ReadCycleStateKey[string](cs, state.CredsRefName)
	require.NoError(t, err)
	require.Equal(t, credName, actualCredsName)

	actualCredsNamespace, err := plugin.ReadCycleStateKey[string](cs, state.CredsRefNamespace)
	require.NoError(t, err)
	require.Equal(t, extNS, actualCredsNamespace)

	actualAPIFormat, err := plugin.ReadCycleStateKey[apiformat.APIFormat](cs, state.APIFormatKey)
	require.NoError(t, err)
	require.Equal(t, apiformat.Messages, actualAPIFormat)

	actualAuthType, err := plugin.ReadCycleStateKey[auth.Auth](cs, state.AuthTypeKey)
	require.NoError(t, err)
	require.Equal(t, auth.APIKey, actualAuthType)

	actualEndpoint, err := plugin.ReadCycleStateKey[string](cs, state.EndpointKey)
	require.NoError(t, err)
	require.Equal(t, endpoint, actualEndpoint)
}

func TestProcessRequest_PathWrittenToCycleState(t *testing.T) {
	store := newInfoStore()
	const (
		extNS       = "llm"
		extName     = "remote-llama"
		targetModel = "llama-4-scout"
		credName    = "cluster-b-key"
		endpoint    = "maas.cluster-b.example.com"
		path        = "/maas-default-gateway/v1/chat/completions"
	)
	store.addOrUpdateModel(extName,
		&externalModelInfo{modelName: extName, refs: []*resolvedProviderRef{{
			provider:        provider.OpenAI,
			targetModel:     targetModel,
			apiFormat:       apiformat.OpenAIChatCompletions,
			auth:            auth.APIKey,
			endpoint:        endpoint,
			path:            path,
			secretName:      credName,
			secretNamespace: extNS,
			config:          map[string]string{},
			weight:          1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/" + extNS + "/" + extName + "/v1/chat/completions"
	req.Body["model"] = extName

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err)

	actualPath, err := plugin.ReadCycleStateKey[string](cs, state.PathKey)
	require.NoError(t, err)
	require.Equal(t, path, actualPath)
}

func TestProcessRequest_UnknownModelPassesThrough(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("gpt4",
		&externalModelInfo{modelName: "gpt4", refs: []*resolvedProviderRef{{
			provider: provider.OpenAI, targetModel: "gpt-4o",
			apiFormat: apiformat.OpenAIChatCompletions,
			secretName: "k", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)
	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/gpt4/v1/chat/completions"
	req.Body["model"] = "unknown-model"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err, "unknown model name should pass through for internal models")

	_, provErr := plugin.ReadCycleStateKey[string](cs, state.ProviderKey)
	require.Error(t, provErr, "provider should not be set for unknown models")
}

func TestProcessRequest_ModelNotFound(t *testing.T) {
	store := newInfoStore()
	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/model-ns/model-name/v1/chat/completions"
	req.Body["model"] = "unknown-model"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err)

	_, err = plugin.ReadCycleStateKey[string](cs, state.ProviderKey)
	require.Error(t, err)
}

func TestProcessRequest_NoModel(t *testing.T) {
	store := newInfoStore()
	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()

	err := instance.ProcessRequest(context.Background(), cs, requesthandling.NewInferenceRequest())
	require.NoError(t, err)

	_, err = plugin.ReadCycleStateKey[string](cs, state.ProviderKey)
	require.Error(t, err)
	_, err = plugin.ReadCycleStateKey[string](cs, state.ModelKey)
	require.Error(t, err)
}

func TestProcessRequest_BadPath(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("ext",
		&externalModelInfo{modelName: "ext", refs: []*resolvedProviderRef{{
			provider: provider.OpenAI, targetModel: "gpt-4o",
			secretName: "k", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)
	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/incomplete"
	req.Body["model"] = "gpt-4o"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err)

	_, err = plugin.ReadCycleStateKey[string](cs, state.ProviderKey)
	require.Error(t, err)
}

func TestSelectByWeight_SingleRef(t *testing.T) {
	refs := []*resolvedProviderRef{
		{provider: "openai", weight: 1},
	}
	selected := selectByWeight(refs)
	require.Equal(t, "openai", selected.provider)
}

func TestSelectByWeight_Distribution(t *testing.T) {
	refs := []*resolvedProviderRef{
		{provider: "openai", weight: 80},
		{provider: "anthropic", weight: 20},
	}

	counts := map[string]int{}
	for range 1000 {
		selected := selectByWeight(refs)
		counts[selected.provider]++
	}

	require.Greater(t, counts["openai"], 700, "openai should get majority of traffic")
	require.Greater(t, counts["anthropic"], 100, "anthropic should get some traffic")
}

func TestSelectByWeight_EqualWeights(t *testing.T) {
	refs := []*resolvedProviderRef{
		{provider: "a", weight: 1},
		{provider: "b", weight: 1},
		{provider: "c", weight: 1},
	}

	counts := map[string]int{}
	for range 900 {
		selected := selectByWeight(refs)
		counts[selected.provider]++
	}

	for _, p := range []string{"a", "b", "c"} {
		require.Greater(t, counts[p], 200, "%s should get roughly equal traffic", p)
	}
}

func TestProcessRequest_AnthropicMessages(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("claude",
		&externalModelInfo{modelName: "claude", refs: []*resolvedProviderRef{{
			provider: provider.Anthropic, targetModel: "claude-opus-4-6",
			apiFormat: "messages", secretName: "key", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/claude/v1/messages"
	req.Body["model"] = "claude"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err)

	inputFmt, err := plugin.ReadCycleStateKey[apiformat.APIFormat](cs, state.InputAPIFormatKey)
	require.NoError(t, err)
	require.Equal(t, apiformat.Messages, inputFmt)

	apiFormat, err := plugin.ReadCycleStateKey[apiformat.APIFormat](cs, state.APIFormatKey)
	require.NoError(t, err)
	require.Equal(t, apiformat.Messages, apiFormat)
}

func TestProcessRequest_OpenAIResponses(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("gpt",
		&externalModelInfo{modelName: "gpt", refs: []*resolvedProviderRef{{
			provider: provider.OpenAI, targetModel: "gpt-5.5",
			apiFormat: "openai-chat", secretName: "key", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/gpt/v1/responses"
	req.Body["model"] = "gpt"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err)

	inputFmt, err := plugin.ReadCycleStateKey[apiformat.APIFormat](cs, state.InputAPIFormatKey)
	require.NoError(t, err)
	require.Equal(t, apiformat.OpenAIResponses, inputFmt)
}

func TestProcessRequest_UnsupportedPath(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("model",
		&externalModelInfo{modelName: "model", refs: []*resolvedProviderRef{{
			provider: provider.OpenAI, targetModel: "gpt-4o",
			apiFormat: "openai-chat", secretName: "key", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/model/v1/unknown"
	req.Body["model"] = "model"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported API endpoint")
}

// --- Loop detection tests ---

func TestProcessRequest_ForwardedRequest_Blocked(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("remote-llama",
		&externalModelInfo{modelName: "remote-llama", refs: []*resolvedProviderRef{{
			provider: provider.OpenAI, targetModel: "llama-4-scout",
			apiFormat: apiformat.OpenAIChatCompletions, auth: auth.APIKey,
			endpoint: "maas.cluster-b.example.com", path: "/maas/v1/chat/completions",
			secretName: "key", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/remote-llama/v1/chat/completions"
	req.Headers[ForwardedHeader] = ForwardedHeaderValue
	req.Body["model"] = "remote-llama"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "routing loop detected")
}

func TestProcessRequest_ForwardedRequest_BeforeCycleStateWrite(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("remote-llama",
		&externalModelInfo{modelName: "remote-llama", refs: []*resolvedProviderRef{{
			provider: provider.OpenAI, targetModel: "llama-4-scout",
			apiFormat: apiformat.OpenAIChatCompletions, auth: auth.APIKey,
			endpoint: "maas.cluster-b.example.com", path: "/maas/v1/chat/completions",
			secretName: "key", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/remote-llama/v1/chat/completions"
	req.Headers[ForwardedHeader] = ForwardedHeaderValue
	req.Body["model"] = "remote-llama"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.Error(t, err)

	_, csErr := plugin.ReadCycleStateKey[string](cs, state.ProviderKey)
	require.Error(t, csErr, "CycleState should be empty — loop rejected before CycleState writes")
}

func TestProcessRequest_ForwardedRequest_InternalModel_PassesThrough(t *testing.T) {
	store := newInfoStore()

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/local-model/v1/chat/completions"
	req.Headers[ForwardedHeader] = ForwardedHeaderValue
	req.Body["model"] = "local-model"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err, "forwarded request to an internal model (not in ExternalModel store) should pass through")
}

func TestProcessRequest_NoForwardedHeader_Passes(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("gpt4",
		&externalModelInfo{modelName: "gpt4", refs: []*resolvedProviderRef{{
			provider: provider.OpenAI, targetModel: "gpt-4o",
			apiFormat: apiformat.OpenAIChatCompletions, auth: auth.APIKey,
			endpoint: "api.openai.com",
			secretName: "key", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/gpt4/v1/chat/completions"
	req.Body["model"] = "gpt4"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err, "request without forwarded header should pass")

	actualProvider, err := plugin.ReadCycleStateKey[string](cs, state.ProviderKey)
	require.NoError(t, err)
	require.Equal(t, provider.OpenAI, actualProvider)
}

func TestProcessRequest_ExternalModel_InjectsForwardedHeader(t *testing.T) {
	store := newInfoStore()
	store.addOrUpdateModel("gpt4",
		&externalModelInfo{modelName: "gpt4", refs: []*resolvedProviderRef{{
			provider: provider.OpenAI, targetModel: "gpt-4o",
			apiFormat: apiformat.OpenAIChatCompletions, auth: auth.APIKey,
			endpoint: "api.openai.com",
			secretName: "key", secretNamespace: "llm",
			config: map[string]string{}, weight: 1,
		}}},
	)

	instance := &ModelProviderResolverPlugin{store: store}
	cs := plugin.NewCycleState()
	req := requesthandling.NewInferenceRequest()
	req.Headers[":path"] = "/llm/gpt4/v1/chat/completions"
	req.Body["model"] = "gpt4"

	err := instance.ProcessRequest(context.Background(), cs, req)
	require.NoError(t, err)

	mutated := req.MutatedHeaders()
	require.Equal(t, ForwardedHeaderValue, mutated[ForwardedHeader],
		"ExternalModel resolution should inject x-gateway-forwarded header")
}

func TestDetectInputAPIFormat(t *testing.T) {
	tests := []struct {
		path     string
		expected apiformat.APIFormat
	}{
		{"llm/model/v1/chat/completions", apiformat.OpenAIChatCompletions},
		{"llm/model/v1/messages", apiformat.Messages},
		{"llm/model/v1/responses", apiformat.OpenAIResponses},
		{"llm/model/v1/unknown", ""},
		{"llm/model/v1/models", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := detectInputAPIFormat(tt.path)
			require.Equal(t, tt.expected, result)
		})
	}
}

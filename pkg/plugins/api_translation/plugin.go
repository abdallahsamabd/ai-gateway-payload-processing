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

package api_translation

import (
	"context"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/gateway-api-inference-extension/pkg/bbr/framework"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/plugin"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/api_translation/providers"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/api_translation/providers/anthropic"
)

const (
	// PluginType is the unique identifier for this plugin, used in CLI flags and registry.
	PluginType = "api-translation"

	// CycleState keys
	cycleStateProviderKey = "provider"             // read from CycleState, set by upstream plugin
	cycleStateModelKey    = "api-translation-model" // written by this plugin for response use
)

// compile-time type validation
var _ framework.RequestProcessor = &APITranslationPlugin{}
var _ framework.ResponseProcessor = &APITranslationPlugin{}

// APITranslationPlugin translates inference API requests and responses between
// OpenAI Chat Completions format and provider-native formats (e.g., Anthropic Messages API).
type APITranslationPlugin struct {
	typedName plugin.TypedName
	providers map[string]providers.Provider
}

// NewAPITranslationPlugin creates a new plugin instance with all registered providers.
func NewAPITranslationPlugin() (*APITranslationPlugin, error) {
	return &APITranslationPlugin{
		typedName: plugin.TypedName{
			Type: PluginType,
			Name: PluginType,
		},
		providers: map[string]providers.Provider{
			anthropic.ProviderName: anthropic.NewAnthropicProvider(),
		},
	}, nil
}

// Factory is the factory function for creating APITranslationPlugin instances.
func Factory(name string, _ json.RawMessage, _ framework.Handle) (framework.BBRPlugin, error) {
	p, err := NewAPITranslationPlugin()
	if err != nil {
		return nil, fmt.Errorf("failed to create '%s' plugin - %w", PluginType, err)
	}
	return p.WithName(name), nil
}

// TypedName returns the type and name tuple of this plugin instance.
func (p *APITranslationPlugin) TypedName() plugin.TypedName {
	return p.typedName
}

// WithName sets the name of the plugin instance.
func (p *APITranslationPlugin) WithName(name string) *APITranslationPlugin {
	p.typedName.Name = name
	return p
}

// ProcessRequest reads the provider from CycleState (set by an upstream plugin) and translates
// the request body from OpenAI format to the provider's native format if needed.
func (p *APITranslationPlugin) ProcessRequest(ctx context.Context, cycleState *framework.CycleState, request *framework.InferenceRequest) error {
	if request == nil || request.Headers == nil || request.Body == nil {
		return fmt.Errorf("invalid inference request: request/headers/body must be non-nil")
	}

	providerName, _ := framework.ReadCycleStateKey[string](cycleState, cycleStateProviderKey)
	if providerName == "" || providerName == "openai" {
		return nil
	}

	translator, ok := p.providers[providerName]
	if !ok {
		return fmt.Errorf("unsupported provider %q", providerName)
	}

	// Store model in CycleState for response use
	if model, ok := request.Body["model"].(string); ok {
		cycleState.Write(cycleStateModelKey, model)
	}

	translatedBody, headers, headersToRemove, err := translator.TranslateRequest(request.Body)
	if err != nil {
		return fmt.Errorf("request translation failed for provider %q: %w", providerName, err)
	}

	if translatedBody != nil {
		request.SetBody(translatedBody)
	}

	for k, v := range headers {
		request.SetHeader(k, v)
	}
	for _, k := range headersToRemove {
		request.RemoveHeader(k)
	}

	return nil
}

// ProcessResponse reads the provider from CycleState and translates the response
// back to OpenAI Chat Completions format if needed.
func (p *APITranslationPlugin) ProcessResponse(ctx context.Context, cycleState *framework.CycleState, response *framework.InferenceResponse) error {
	if response == nil || response.Headers == nil || response.Body == nil {
		return fmt.Errorf("invalid inference response: response/headers/body must be non-nil")
	}

	providerName, _ := framework.ReadCycleStateKey[string](cycleState, cycleStateProviderKey)
	if providerName == "" || providerName == "openai" {
		return nil
	}

	translator, ok := p.providers[providerName]
	if !ok {
		return nil
	}

	model, _ := framework.ReadCycleStateKey[string](cycleState, cycleStateModelKey)

	translatedBody, err := translator.TranslateResponse(response.Body, model)
	if err != nil {
		return fmt.Errorf("response translation failed for provider %q: %w", providerName, err)
	}

	if translatedBody != nil {
		response.SetBody(translatedBody)
	}

	return nil
}

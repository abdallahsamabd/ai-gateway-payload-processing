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
	"encoding/json"
	"fmt"

	"sigs.k8s.io/gateway-api-inference-extension/pkg/bbr/framework"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/plugin"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/inference_api_translator/providers"
)

const (
	// PluginType is the unique identifier for this plugin, used in CLI flags and registry.
	PluginType = "inference-api-translator"

	// ProviderHeader is the request header that indicates the target provider.
	// Set by an upstream plugin in the BBR chain (e.g., api-key-injection).
	// Absent or "openai" means no translation needed.
	ProviderHeader = "X-Gateway-Destination-Provider"
)

// compile-time type validation
var _ framework.RequestProcessor = &InferenceAPITranslatorPlugin{}
var _ framework.ResponseProcessor = &InferenceAPITranslatorPlugin{}

// InferenceAPITranslatorPlugin translates inference API requests and responses between
// OpenAI Chat Completions format and provider-native formats (e.g., Anthropic Messages API).
type InferenceAPITranslatorPlugin struct {
	typedName     plugin.TypedName
	providerIndex map[string]providers.Provider
}

// NewInferenceAPITranslatorPlugin creates a new plugin instance with all registered providers.
func NewInferenceAPITranslatorPlugin() (*InferenceAPITranslatorPlugin, error) {
	providerMap := map[string]providers.Provider{}
	for _, p := range []providers.Provider{
		providers.NewAnthropicProvider(),
		providers.NewOpenAIProvider(),
	} {
		providerMap[p.Name()] = p
	}

	return &InferenceAPITranslatorPlugin{
		typedName: plugin.TypedName{
			Type: PluginType,
			Name: PluginType,
		},
		providerIndex: providerMap,
	}, nil
}

// Factory is the factory function for creating InferenceAPITranslatorPlugin instances.
func Factory(name string, _ json.RawMessage) (framework.BBRPlugin, error) {
	p, err := NewInferenceAPITranslatorPlugin()
	if err != nil {
		return nil, fmt.Errorf("failed to create '%s' plugin - %w", PluginType, err)
	}
	return p.WithName(name), nil
}

// TypedName returns the type and name tuple of this plugin instance.
func (p *InferenceAPITranslatorPlugin) TypedName() plugin.TypedName {
	return p.typedName
}

// WithName sets the name of the plugin instance.
func (p *InferenceAPITranslatorPlugin) WithName(name string) *InferenceAPITranslatorPlugin {
	p.typedName.Name = name
	return p
}

// ProcessRequest checks the X-Gateway-Destination-Provider header and translates
// the request body from OpenAI format to the provider's native format if needed.
func (p *InferenceAPITranslatorPlugin) ProcessRequest(ctx context.Context, request *framework.InferenceRequest) error {
	if request == nil || request.Headers == nil || request.Body == nil {
		return fmt.Errorf("invalid inference request: request/headers/body must be non-nil")
	}

	providerName := request.Headers[ProviderHeader]
	if providerName == "" || providerName == "openai" {
		return nil
	}

	provider, ok := p.providerIndex[providerName]
	if !ok {
		return fmt.Errorf("unsupported provider %q in header %q", providerName, ProviderHeader)
	}

	translatedBody, headers, headersToRemove, err := provider.TranslateRequest(request.Body)
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

// ProcessResponse detects the provider from the response body format and translates
// the response back to OpenAI Chat Completions format if needed.
func (p *InferenceAPITranslatorPlugin) ProcessResponse(ctx context.Context, response *framework.InferenceResponse) error {
	if response == nil || response.Headers == nil || response.Body == nil {
		return fmt.Errorf("invalid inference response: response/headers/body must be non-nil")
	}

	providerName := detectProviderFromResponse(response.Body)
	if providerName == "" || providerName == "openai" {
		return nil
	}

	provider, ok := p.providerIndex[providerName]
	if !ok {
		return nil
	}

	model, _ := response.Body["model"].(string)

	translatedBody, err := provider.TranslateResponse(response.Body, model)
	if err != nil {
		return fmt.Errorf("response translation failed for provider %q: %w", providerName, err)
	}

	if translatedBody != nil {
		response.SetBody(translatedBody)
	}

	return nil
}

// detectProviderFromResponse identifies the provider from the response body structure.
// Anthropic success responses have type="message"; error responses have type="error".
// OpenAI responses have object="chat.completion"; errors have a top-level "error" key.
func detectProviderFromResponse(body map[string]any) string {
	if bodyType, ok := body["type"].(string); ok {
		if bodyType == "message" || bodyType == "error" {
			return "anthropic"
		}
	}
	if _, ok := body["object"].(string); ok {
		return "openai"
	}
	if _, ok := body["error"]; ok {
		return "openai"
	}
	return ""
}

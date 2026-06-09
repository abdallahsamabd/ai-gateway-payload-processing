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

package vertex

import (
	"fmt"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/api-translation/translator"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/api-translation/translator/anthropic"
)

const (
	// AnthropicVersionConfigKey is the key in the ExternalModel/Provider config map
	// that carries the Anthropic API version for Vertex AI Claude requests.
	AnthropicVersionConfigKey = "anthropicVersion"
)

// compile-time interface checks
var _ translator.Translator = &VertexAnthropicTranslator{}
var _ translator.ConfigAwareTranslator = &VertexAnthropicTranslator{}

// NewVertexAnthropicTranslator creates a translator for Vertex AI's Anthropic Claude endpoint.
// Path construction is handled by CRD path placeholders and applyPathOverride, not by the translator.
func NewVertexAnthropicTranslator() *VertexAnthropicTranslator {
	return &VertexAnthropicTranslator{
		anthropic: anthropic.NewAnthropicTranslator(),
	}
}

// VertexAnthropicTranslator translates OpenAI Chat Completions to Anthropic Messages API
// format for Claude models on Google Cloud Vertex AI. Body transformation reuses
// AnthropicTranslator; Vertex-specific differences are anthropic_version in the body
// (not header) and model removed from body (path is set by applyPathOverride from CRD config).
type VertexAnthropicTranslator struct {
	anthropic *anthropic.AnthropicTranslator
}

// TranslateRequest satisfies the base Translator interface but should not be called directly.
// Use TranslateRequestWithConfig instead; the plugin dispatches via ConfigAwareTranslator.
func (t *VertexAnthropicTranslator) TranslateRequest(body map[string]any) (map[string]any, map[string]string, []string, error) {
	return nil, nil, nil, fmt.Errorf("vertex-anthropic requires config; use TranslateRequestWithConfig")
}

// TranslateRequestWithConfig delegates to AnthropicTranslator for body transformation, then
// applies Vertex-specific adjustments: removes model from body (path is set by applyPathOverride),
// injects anthropic_version from config, removes anthropic-version header.
func (t *VertexAnthropicTranslator) TranslateRequestWithConfig(body map[string]any, config map[string]string) (map[string]any, map[string]string, []string, error) {
	translatedBody, _, headersToRemove, err := t.anthropic.TranslateRequest(body)
	if err != nil {
		return nil, nil, nil, err
	}

	delete(translatedBody, "model")

	anthropicVersion := config[AnthropicVersionConfigKey]
	if anthropicVersion == "" {
		return nil, nil, nil, fmt.Errorf("%s is required in ExternalModel/Provider config for vertex-anthropic", AnthropicVersionConfigKey)
	}
	translatedBody["anthropic_version"] = anthropicVersion

	headers := map[string]string{
		"content-type": "application/json",
	}

	return translatedBody, headers, headersToRemove, nil
}

// TranslateResponse delegates to AnthropicTranslator since Vertex returns the same
// Anthropic Messages API response format.
func (t *VertexAnthropicTranslator) TranslateResponse(body map[string]any, model string) (map[string]any, error) {
	return t.anthropic.TranslateResponse(body, model)
}

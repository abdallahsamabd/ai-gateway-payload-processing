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

package azure

import (
	"fmt"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/api-translation/translator"
)

const (
	// Azure OpenAI v1 API chat completions path.
	// Reference: https://learn.microsoft.com/en-us/azure/foundry/openai/latest#create-chat-completion
	azureChatCompletionsPath = "/openai/v1/chat/completions"
)

// compile-time interface check
var _ translator.Translator = &AzureOpenAITranslator{}

// NewAzureOpenAITranslator initializes a new AzureOpenAITranslator and returns its pointer.
func NewAzureOpenAITranslator() *AzureOpenAITranslator {
	return &AzureOpenAITranslator{}
}

// AzureOpenAITranslator translates between OpenAI Chat Completions format and
// Azure OpenAI Service format. Azure OpenAI uses the same request/response schema
// as OpenAI, so translation is limited to path rewriting and header adjustments.
type AzureOpenAITranslator struct{}

// TranslateRequest rewrites the path and headers for Azure OpenAI v1 API.
// The request body is not mutated since Azure OpenAI accepts the same schema as OpenAI.
func (t *AzureOpenAITranslator) TranslateRequest(body map[string]any) (map[string]any, map[string]string, []string, error) {
	model, _ := body["model"].(string)
	if model == "" {
		return nil, nil, nil, fmt.Errorf("model field is required")
	}

	headers := map[string]string{
		":path":        azureChatCompletionsPath,
		"content-type": "application/json",
	}

	return nil, headers, nil, nil
}

// TranslateResponse strips Azure-specific fields (content_filter_results, prompt_filter_results)
// from the response body, returning a clean OpenAI-compatible response.
func (t *AzureOpenAITranslator) TranslateResponse(body map[string]any, model string) (map[string]any, error) {
	mutated := false

	if _, ok := body["prompt_filter_results"]; ok {
		delete(body, "prompt_filter_results")
		mutated = true
	}

	if choices, ok := body["choices"].([]any); ok {
		for _, raw := range choices {
			if choice, ok := raw.(map[string]any); ok {
				if _, ok := choice["content_filter_results"]; ok {
					delete(choice, "content_filter_results")
					mutated = true
				}
			}
		}
	}

	if !mutated {
		return nil, nil
	}
	return body, nil
}

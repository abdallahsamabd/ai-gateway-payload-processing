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
	"regexp"
	"strings"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/api-translation/translator"
)

const (
	// defaultAPIVersion is the default Azure OpenAI API version.
	// Reference: https://learn.microsoft.com/en-us/azure/ai-services/openai/reference
	defaultAPIVersion = "2024-10-21"

	// Azure OpenAI endpoint path template.
	// The deployment ID typically matches the deployed model name.
	// Reference: https://learn.microsoft.com/en-us/azure/ai-foundry/openai/reference#chat-completions
	azurePathTemplate = "/openai/deployments/%s/chat/completions?api-version=%s"
)

// DefaultResponseFieldsToStrip is the default set of Azure-specific response fields to remove.
// Paths use dot-separated segments; a "[]" suffix means "iterate array elements."
//
// Examples:
//
//	"prompt_filter_results"            → deletes the top-level key
//	"choices[].content_filter_results" → iterates choices and deletes from each element
var DefaultResponseFieldsToStrip = []string{
	"prompt_filter_results",
	"choices[].content_filter_results",
}

// compile-time interface check
var _ translator.Translator = &AzureOpenAITranslator{}

// NewAzureOpenAITranslator initializes a new AzureOpenAITranslator.
// responseFieldsToStrip defines which provider-specific fields to remove from responses,
// using dot-separated paths with "[]" to denote array traversal.
// Pass nil or an empty slice for no stripping (no-op response translation).
func NewAzureOpenAITranslator(responseFieldsToStrip []string) *AzureOpenAITranslator {
	return &AzureOpenAITranslator{
		apiVersion:          defaultAPIVersion,
		deploymentIDPattern: regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`),
		fieldPaths:          parseFieldPaths(responseFieldsToStrip),
	}
}

// AzureOpenAITranslator translates between OpenAI Chat Completions format and
// Azure OpenAI Service format. Azure OpenAI uses the same request/response schema
// as OpenAI, so translation is limited to path rewriting, header adjustments,
// and stripping provider-specific response fields.
type AzureOpenAITranslator struct {
	apiVersion          string
	deploymentIDPattern *regexp.Regexp
	fieldPaths          []fieldPath
}

// TranslateRequest rewrites the path and headers for Azure OpenAI.
// The request body is not mutated since Azure OpenAI accepts the same schema as OpenAI.
// Azure ignores the model field in the body and uses the deployment ID from the URI path.
func (t *AzureOpenAITranslator) TranslateRequest(body map[string]any) (map[string]any, map[string]string, []string, error) {
	model, _ := body["model"].(string)
	if model == "" {
		return nil, nil, nil, fmt.Errorf("model field is required")
	}
	if !t.deploymentIDPattern.MatchString(model) {
		return nil, nil, nil, fmt.Errorf("model '%s' contains invalid characters for Azure deployment ID", model)
	}

	headers := map[string]string{
		":path":        fmt.Sprintf(azurePathTemplate, model, t.apiVersion),
		"content-type": "application/json",
	}

	// Return nil body — no body mutation is needed, Azure accepts the OpenAI request format as-is.
	return nil, headers, nil, nil
}

// TranslateResponse walks the configured field paths and strips matching
// provider-specific fields from the response body.
func (t *AzureOpenAITranslator) TranslateResponse(body map[string]any, model string) (map[string]any, error) {
	mutated := false
	for _, fp := range t.fieldPaths {
		if stripField(body, fp, 0) {
			mutated = true
		}
	}
	if !mutated {
		return nil, nil
	}
	return body, nil
}

// --- field-path helpers ---

// fieldSegment represents one segment of a dot-separated strip path.
type fieldSegment struct {
	key     string
	isArray bool // true when the segment had a "[]" suffix
}

// fieldPath is a parsed sequence of segments.
type fieldPath []fieldSegment

// parseFieldPaths converts raw dot-separated path strings into parsed fieldPaths.
func parseFieldPaths(raw []string) []fieldPath {
	paths := make([]fieldPath, 0, len(raw))
	for _, r := range raw {
		parts := strings.Split(r, ".")
		segments := make(fieldPath, 0, len(parts))
		for _, p := range parts {
			if strings.HasSuffix(p, "[]") {
				segments = append(segments, fieldSegment{key: strings.TrimSuffix(p, "[]"), isArray: true})
			} else {
				segments = append(segments, fieldSegment{key: p})
			}
		}
		paths = append(paths, segments)
	}
	return paths
}

// stripField recursively walks the body according to the parsed path and deletes
// the leaf key. Returns true if at least one deletion occurred.
func stripField(obj map[string]any, path fieldPath, idx int) bool {
	if idx >= len(path) {
		return false
	}

	seg := path[idx]
	isLast := idx == len(path)-1

	if isLast {
		if _, ok := obj[seg.key]; ok {
			delete(obj, seg.key)
			return true
		}
		return false
	}

	// Intermediate segment — descend into map or iterate array.
	if seg.isArray {
		arr, ok := obj[seg.key].([]any)
		if !ok {
			return false
		}
		mutated := false
		for _, elem := range arr {
			if m, ok := elem.(map[string]any); ok {
				if stripField(m, path, idx+1) {
					mutated = true
				}
			}
		}
		return mutated
	}

	child, ok := obj[seg.key].(map[string]any)
	if !ok {
		return false
	}
	return stripField(child, path, idx+1)
}

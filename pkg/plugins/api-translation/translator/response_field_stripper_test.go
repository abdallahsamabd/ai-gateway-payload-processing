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

package translator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrip_TopLevelField(t *testing.T) {
	body := map[string]any{
		"id":                    "chatcmpl-abc123",
		"object":                "chat.completion",
		"prompt_filter_results": []any{map[string]any{"prompt_index": float64(0)}},
	}

	stripper := NewResponseFieldStripper([]string{"prompt_filter_results"})
	result, mutated := stripper.Strip(body)

	require.True(t, mutated)
	require.NotNil(t, result)
	assert.NotContains(t, result, "prompt_filter_results")
	assert.Equal(t, "chatcmpl-abc123", result["id"])
}

func TestStrip_ArrayElementField(t *testing.T) {
	body := map[string]any{
		"choices": []any{
			map[string]any{
				"index":                  float64(0),
				"content_filter_results": map[string]any{"hate": "safe"},
				"message":                map[string]any{"role": "assistant", "content": "Hi"},
			},
			map[string]any{
				"index":                  float64(1),
				"content_filter_results": map[string]any{"hate": "safe"},
				"message":                map[string]any{"role": "assistant", "content": "Hello"},
			},
		},
	}

	stripper := NewResponseFieldStripper([]string{"choices[].content_filter_results"})
	result, mutated := stripper.Strip(body)

	require.True(t, mutated)
	require.NotNil(t, result)
	choices := result["choices"].([]any)
	for i, raw := range choices {
		choice := raw.(map[string]any)
		assert.NotContains(t, choice, "content_filter_results", "choices[%d]", i)
		assert.Contains(t, choice, "message", "choices[%d]", i)
	}
}

func TestStrip_NestedMapPath(t *testing.T) {
	body := map[string]any{
		"id":     "chatcmpl-abc123",
		"object": "chat.completion",
		"metadata": map[string]any{
			"internal_id": "xyz",
			"debug_info":  "should-be-stripped",
			"region":      "eastus",
		},
	}

	stripper := NewResponseFieldStripper([]string{"metadata.debug_info"})
	result, mutated := stripper.Strip(body)

	require.True(t, mutated)
	require.NotNil(t, result)
	metadata := result["metadata"].(map[string]any)
	assert.NotContains(t, metadata, "debug_info")
	assert.Equal(t, "xyz", metadata["internal_id"])
	assert.Equal(t, "eastus", metadata["region"])
}

func TestStrip_MultiplePaths(t *testing.T) {
	body := map[string]any{
		"id":     "chatcmpl-abc123",
		"object": "chat.completion",
		"choices": []any{
			map[string]any{
				"index":                  float64(0),
				"content_filter_results": map[string]any{"hate": "safe"},
				"custom_provider_field":  "should-be-stripped",
				"message":                map[string]any{"role": "assistant", "content": "Hello!"},
			},
		},
		"prompt_filter_results": []any{map[string]any{"prompt_index": float64(0)}},
		"system_fingerprint":    "fp_abc123",
	}

	stripper := NewResponseFieldStripper([]string{
		"system_fingerprint",
		"choices[].custom_provider_field",
	})
	result, mutated := stripper.Strip(body)

	require.True(t, mutated)
	require.NotNil(t, result)

	assert.NotContains(t, result, "system_fingerprint")
	assert.Contains(t, result, "prompt_filter_results", "only configured paths should be stripped")

	choices := result["choices"].([]any)
	choice := choices[0].(map[string]any)
	assert.NotContains(t, choice, "custom_provider_field")
	assert.Contains(t, choice, "content_filter_results", "only configured paths should be stripped")
	assert.Equal(t, "Hello!", choice["message"].(map[string]any)["content"])
}

func TestStrip_NilPaths(t *testing.T) {
	body := map[string]any{
		"id":                    "chatcmpl-abc123",
		"prompt_filter_results": []any{map[string]any{"prompt_index": float64(0)}},
	}

	stripper := NewResponseFieldStripper(nil)
	result, mutated := stripper.Strip(body)

	assert.False(t, mutated)
	assert.Nil(t, result, "nil paths should result in no-op")
}

func TestStrip_EmptyPaths(t *testing.T) {
	body := map[string]any{
		"id":                    "chatcmpl-abc123",
		"prompt_filter_results": []any{map[string]any{"prompt_index": float64(0)}},
	}

	stripper := NewResponseFieldStripper([]string{})
	result, mutated := stripper.Strip(body)

	assert.False(t, mutated)
	assert.Nil(t, result, "empty paths should result in no-op")
}

func TestStrip_NoMatchingFields(t *testing.T) {
	body := map[string]any{
		"id":     "chatcmpl-abc123",
		"object": "chat.completion",
	}

	stripper := NewResponseFieldStripper([]string{"nonexistent_field", "choices[].missing"})
	result, mutated := stripper.Strip(body)

	assert.False(t, mutated)
	assert.Nil(t, result)
}

func TestStrip_EmptyBody(t *testing.T) {
	body := map[string]any{}

	stripper := NewResponseFieldStripper([]string{"prompt_filter_results"})
	result, mutated := stripper.Strip(body)

	assert.False(t, mutated)
	assert.Nil(t, result)
}

func TestStrip_ArrayWithMixedTypes(t *testing.T) {
	body := map[string]any{
		"choices": []any{
			map[string]any{
				"index":       float64(0),
				"extra_field": "strip-me",
			},
			"not-a-map",
			float64(42),
			nil,
			map[string]any{
				"index":       float64(1),
				"extra_field": "strip-me-too",
			},
		},
	}

	stripper := NewResponseFieldStripper([]string{"choices[].extra_field"})
	result, mutated := stripper.Strip(body)

	require.True(t, mutated)
	require.NotNil(t, result)
	choices := result["choices"].([]any)
	assert.NotContains(t, choices[0].(map[string]any), "extra_field")
	assert.Equal(t, "not-a-map", choices[1])
	assert.Equal(t, float64(42), choices[2])
	assert.Nil(t, choices[3])
	assert.NotContains(t, choices[4].(map[string]any), "extra_field")
}

func TestStrip_ArrayFieldNotArray(t *testing.T) {
	body := map[string]any{
		"choices": "not-an-array",
	}

	stripper := NewResponseFieldStripper([]string{"choices[].content_filter_results"})
	result, mutated := stripper.Strip(body)

	assert.False(t, mutated)
	assert.Nil(t, result)
}

func TestStrip_NestedFieldNotMap(t *testing.T) {
	body := map[string]any{
		"metadata": "not-a-map",
	}

	stripper := NewResponseFieldStripper([]string{"metadata.debug_info"})
	result, mutated := stripper.Strip(body)

	assert.False(t, mutated)
	assert.Nil(t, result)
}

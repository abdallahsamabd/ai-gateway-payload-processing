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

import "strings"

// ResponseFieldStripper removes provider-specific fields from a response body
// based on configurable dot-separated paths. Any translator that wraps an
// OpenAI-compatible provider can use this to strip extra fields before returning
// the response to the client.
//
// Path format conventions:
//   - "prompt_filter_results"            → deletes a top-level key
//   - "choices[].content_filter_results" → iterates an array and deletes from each element
//   - "metadata.debug_info"              → navigates into a nested object and deletes the leaf key
type ResponseFieldStripper struct {
	fieldPaths []fieldPath
}

// NewResponseFieldStripper creates a stripper that will remove the given field paths
// from response bodies. Paths are parsed once at construction time.
// Pass nil or an empty slice for a no-op stripper.
func NewResponseFieldStripper(fieldsToStrip []string) *ResponseFieldStripper {
	return &ResponseFieldStripper{
		fieldPaths: parseFieldPaths(fieldsToStrip),
	}
}

// Strip walks the configured field paths and removes matching fields from the body.
// Returns the mutated body and true if at least one field was removed.
// If nothing was removed, returns nil and false.
func (s *ResponseFieldStripper) Strip(body map[string]any) (map[string]any, bool) {
	mutated := false
	for _, fp := range s.fieldPaths {
		if stripField(body, fp, 0) {
			mutated = true
		}
	}
	if !mutated {
		return nil, false
	}
	return body, true
}

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

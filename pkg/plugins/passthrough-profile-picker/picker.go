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

package passthrough_profile_picker

import (
	"context"
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log"

	logutil "github.com/llm-d/llm-d-inference-payload-processor/pkg/common/observability/logging"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/plugin"
	"github.com/llm-d/llm-d-inference-payload-processor/pkg/framework/interface/requesthandling"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/apiformat"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/state"
)

const (
	PassthroughProfilePickerType = "passthrough-profile-picker"

	defaultTranslationProfile = "translation"
	defaultPassthroughProfile = "passthrough"
)

var _ requesthandling.ProfilePicker = &PassthroughProfilePicker{}

type config struct {
	TranslationProfile string `json:"translationProfile,omitempty"`
	PassthroughProfile string `json:"passthroughProfile,omitempty"`
}

func Factory(name string, parameters json.RawMessage, _ plugin.Handle) (plugin.Plugin, error) {
	cfg := config{
		TranslationProfile: defaultTranslationProfile,
		PassthroughProfile: defaultPassthroughProfile,
	}
	if len(parameters) > 0 {
		if err := json.Unmarshal(parameters, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse passthrough-profile-picker parameters: %w", err)
		}
	}
	return &PassthroughProfilePicker{
		typedName:          plugin.TypedName{Type: PassthroughProfilePickerType, Name: name},
		translationProfile: cfg.TranslationProfile,
		passthroughProfile: cfg.PassthroughProfile,
	}, nil
}

type PassthroughProfilePicker struct {
	typedName          plugin.TypedName
	translationProfile string
	passthroughProfile string
}

func (p *PassthroughProfilePicker) TypedName() plugin.TypedName { return p.typedName }

func (p *PassthroughProfilePicker) Pick(ctx context.Context, cycleState *plugin.CycleState, _ *requesthandling.InferenceRequest, profiles map[string]*requesthandling.Profile) (*requesthandling.Profile, error) {
	logger := log.FromContext(ctx).V(logutil.DEFAULT)

	inputFormat, _ := plugin.ReadCycleStateKey[apiformat.APIFormat](cycleState, state.InputAPIFormatKey)
	outputFormat, _ := plugin.ReadCycleStateKey[apiformat.APIFormat](cycleState, state.APIFormatKey)

	profileName := p.translationProfile
	if isPassthrough(inputFormat, outputFormat) {
		profileName = p.passthroughProfile
	}

	profile, ok := profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}

	logger.Info("profile selected", "profile", profileName, "inputFormat", inputFormat, "outputFormat", outputFormat)
	return profile, nil
}

func isPassthrough(inputFormat, outputFormat apiformat.APIFormat) bool {
	if inputFormat == "" || outputFormat == "" {
		return true
	}
	if inputFormat != outputFormat {
		return false
	}
	return inputFormat != apiformat.OpenAIChatCompletions
}

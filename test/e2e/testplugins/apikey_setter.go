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

package testplugins

import (
	"context"

	"sigs.k8s.io/gateway-api-inference-extension/pkg/bbr/framework"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/plugin"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/provider"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/state"
)

var _ framework.RequestProcessor = &ApiKeySetterPlugin{}

// ApiKeySetterPlugin is a test-only RequestProcessor that injects an API key
// header based on the provider in CycleState. It mirrors the production
// apikey-injection plugin without requiring K8s Secrets.
type ApiKeySetterPlugin struct {
	apiKey string
}

func NewApiKeySetterPlugin(apiKey string) *ApiKeySetterPlugin {
	return &ApiKeySetterPlugin{apiKey: apiKey}
}

func (p *ApiKeySetterPlugin) TypedName() plugin.TypedName {
	return plugin.TypedName{Type: "apikey-setter-test", Name: "apikey-setter-test"}
}

func (p *ApiKeySetterPlugin) ProcessRequest(_ context.Context, cycleState *framework.CycleState, request *framework.InferenceRequest) error {
	providerName, _ := framework.ReadCycleStateKey[string](cycleState, state.ProviderKey)

	switch providerName {
	case provider.AzureOpenAI:
		request.SetHeader("api-key", p.apiKey)
	case provider.Anthropic:
		request.SetHeader("x-api-key", p.apiKey)
	default:
		request.SetHeader("Authorization", "Bearer "+p.apiKey)
	}
	return nil
}

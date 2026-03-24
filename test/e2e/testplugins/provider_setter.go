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

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/state"
)

var _ framework.RequestProcessor = &ProviderSetterPlugin{}

// ProviderSetterPlugin is a test-only RequestProcessor that unconditionally writes a
// fixed provider name into CycleState. It replaces the production provider-resolver
// plugin (which requires K8s MaaSModelRef CRDs) in e2e tests that run without a
// cluster.
type ProviderSetterPlugin struct {
	provider string
}

func NewProviderSetterPlugin(provider string) *ProviderSetterPlugin {
	return &ProviderSetterPlugin{provider: provider}
}

func (p *ProviderSetterPlugin) TypedName() plugin.TypedName {
	return plugin.TypedName{Type: "provider-setter-test", Name: "provider-setter-test"}
}

func (p *ProviderSetterPlugin) ProcessRequest(_ context.Context, cycleState *framework.CycleState, _ *framework.InferenceRequest) error {
	cycleState.Write(state.ProviderKey, p.provider)
	return nil
}

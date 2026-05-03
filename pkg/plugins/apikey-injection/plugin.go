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

package apikey_injection

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/bbr/framework"
	errcommon "sigs.k8s.io/gateway-api-inference-extension/pkg/common/error"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/common/observability/logging"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/framework/interface/plugin"

	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/apikey-injection/auth"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/provider"
	"github.com/opendatahub-io/ai-gateway-payload-processing/pkg/plugins/common/state"
)

const (
	// APIKeyInjectionPluginType is the registered name for this plugin in the BBR registry.
	APIKeyInjectionPluginType = "apikey-injection"
)

// compile-time interface check
var _ framework.RequestProcessor = &ApiKeyInjectionPlugin{}

// APIKeyInjectionFactory defines the factory function for ApiKeyInjectionPlugin.
func APIKeyInjectionFactory(name string, _ json.RawMessage, handle framework.Handle) (framework.BBRPlugin, error) {
	plugin, err := NewAPIKeyInjectionPlugin(handle.Context(), handle.ReconcilerBuilder, handle.ClientReader())
	if err != nil {
		return nil, fmt.Errorf("failed to create plugin '%s' - %w", APIKeyInjectionPluginType, err)
	}

	return plugin.WithName(name), nil
}

// NewAPIKeyInjectionPlugin creates a new apiKeyInjectionPlugin and returns its pointer.
// It sets up a label-filtered informer cache so only Secrets matching the
// ipp-managed label are listed from the API server; Reconcile still checks the
// label after Get before updating the store.
func NewAPIKeyInjectionPlugin(ctx context.Context, reconcilerBuilder func() *builder.Builder, clientReader client.Reader) (*ApiKeyInjectionPlugin, error) {
	store := newSecretStore()
	reconciler := &secretReconciler{
		Reader: clientReader,
		store:  store,
	}

	filteredCache, err := newFilteredSecretCache(ctx)
	if err != nil {
		return nil, err
	}

	var secretObj client.Object = &corev1.Secret{}
	if err := reconcilerBuilder().
		Named("apikey-injection-secret-watcher").
		WatchesRawSource(
			source.Kind(filteredCache, secretObj, &handler.EnqueueRequestForObject{}),
		).
		Complete(reconciler); err != nil {
		return nil, fmt.Errorf("failed to register Secret reconciler for plugin '%s' - %w", APIKeyInjectionPluginType, err)
	}

	return (&ApiKeyInjectionPlugin{
		typedName: plugin.TypedName{
			Type: APIKeyInjectionPluginType,
			Name: APIKeyInjectionPluginType,
		},
		authHeadersGenerators: map[string]auth.AuthHeadersGenerator{
			provider.OpenAI:      &auth.SimpleAuthGenerator{HeaderName: "Authorization", HeaderValuePrefix: "Bearer "},
			provider.Anthropic:   &auth.SimpleAuthGenerator{HeaderName: "x-api-key"},
			provider.AzureOpenAI: &auth.SimpleAuthGenerator{HeaderName: "api-key"},
			// provider.Vertex uses the native GenerateContent API — not used in 3.4 ExternalModel flow.
			// provider.Vertex:     &auth.SimpleAuthGenerator{HeaderName: "Authorization", HeaderValuePrefix: "Bearer "},
			provider.VertexOpenAI:  &auth.SimpleAuthGenerator{HeaderName: "Authorization", HeaderValuePrefix: "Bearer "},
			provider.BedrockOpenAI: &auth.SimpleAuthGenerator{HeaderName: "Authorization", HeaderValuePrefix: "Bearer "},
		},
		store: store,
	}), nil
}

// ApiKeyInjectionPlugin injects an API key from a Kubernetes Secret into the request headers.
// The Secret is identified by its namespaced name from CycleState. The provider (e.g., openai, anthropic)
// determines which header name and value format are used.
type ApiKeyInjectionPlugin struct {
	typedName             plugin.TypedName
	authHeadersGenerators map[string]auth.AuthHeadersGenerator
	store                 *secretStore
}

// TypedName returns the type and name tuple of this plugin instance.
func (p *ApiKeyInjectionPlugin) TypedName() plugin.TypedName {
	return p.typedName
}

// WithName sets the name of this plugin instance.
func (p *ApiKeyInjectionPlugin) WithName(name string) *ApiKeyInjectionPlugin {
	p.typedName.Name = name
	return p
}

// ProcessRequest reads the credential Secret reference and provider from CycleState (written by model-provider-resolver),
// looks up the API key in the store, and injects provider-specific auth headers into the request.
func (p *ApiKeyInjectionPlugin) ProcessRequest(ctx context.Context, cycleState *framework.CycleState, request *framework.InferenceRequest) error {
	// Check if this is an external model (provider set by model-provider-resolver).
	// Internal models have no provider in CycleState and don't need API key injection.
	providerName, err := framework.ReadCycleStateKey[string](cycleState, state.ProviderKey)
	if err != nil || providerName == "" {
		return nil
	}

	credsName, err := framework.ReadCycleStateKey[string](cycleState, state.CredsRefName)
	if err != nil || credsName == "" {
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("provider '%s' is missing credentialRef", providerName)}
	}
	credsNamespace, err := framework.ReadCycleStateKey[string](cycleState, state.CredsRefNamespace)
	if err != nil || credsNamespace == "" {
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("provider '%s' is missing credentialRef namespace", providerName)}
	}

	secretKey := fmt.Sprintf("%s/%s", credsNamespace, credsName)
	credentials, found := p.store.get(secretKey)
	if !found {
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("provider '%s' credentials not found", providerName)}
	}

	generator, ok := p.authHeadersGenerators[providerName]
	if !ok {
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("unsupported provider - '%s'", providerName)}
	}

	authHeaders, err := generator.GenerateAuthHeaders(credentials)
	if err != nil {
		return errcommon.Error{Code: errcommon.Internal, Msg: fmt.Sprintf("failed to generate auth headers for provider '%s': %v", providerName, err)}
	}

	for headerKey, headerValue := range authHeaders {
		request.SetHeader(headerKey, headerValue)
	}

	log.FromContext(ctx).V(logutil.VERBOSE).Info("auth headers injected", "provider", providerName)
	return nil
}

// newFilteredSecretCache creates a controller-runtime cache that restricts the
// Secret informer to only list/watch Secrets labeled with the managed label.
// This is a defense-in-depth measure: even though the RBAC ClusterRole grants
// broad Secret access (required for cross-namespace watches), the informer
// never fetches or caches unrelated Secrets.
func newFilteredSecretCache(ctx context.Context) (cache.Cache, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get rest config for filtered Secret cache: %w", err)
	}

	filteredCache, err := cache.New(cfg, cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Secret{}: {
				Label: labels.SelectorFromSet(labels.Set{
					managedLabel: "true",
				}),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create filtered Secret cache: %w", err)
	}

	go func() {
		if err := filteredCache.Start(ctx); err != nil {
			ctrl.Log.WithName(APIKeyInjectionPluginType).Error(err, "filtered Secret cache stopped unexpectedly")
		}
	}()

	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if !filteredCache.WaitForCacheSync(syncCtx) {
		return nil, fmt.Errorf("filtered Secret cache failed to sync within deadline")
	}

	return filteredCache, nil
}

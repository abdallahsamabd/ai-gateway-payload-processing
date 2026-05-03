# Payload-Processing

A chart to deploy payload-processing.

<!-- TODO we should pin to odh payload processing released tag -->

## RBAC and Secrets Access

The `apikey-injection` plugin watches Kubernetes Secrets to inject provider API
keys (e.g. OpenAI, Anthropic) into outbound inference requests. Only Secrets
labeled `inference.llm-d.ai/ipp-managed: "true"` are processed.

### Single-namespace mode (`multiNamespace: false`, the default)

A **Role** and **RoleBinding** are created in the release namespace. This matches
[Kubernetes RBAC good practices](https://kubernetes.io/docs/concepts/security/rbac-good-practices/)
(least privilege, prefer namespace-scoped bindings) and limits exposure called
out in [#201](https://github.com/opendatahub-io/ai-gateway-payload-processing/issues/201).

Keep `ExternalModel` resources and their credential `Secret`s in the **same
namespace as the Helm release** when using this default. If IPP runs in the
release namespace but models or credentials live in **other** namespaces, set
`upstreamBbr.bbr.multiNamespace: true` in your values (ClusterRole path);
otherwise the controller cannot read those objects. Downstream distributions
that centralize IPP often ship this override in their own values overlay.

### Multi-namespace mode (`multiNamespace: true`, opt-in)

A **ClusterRole** is created with `get`/`list`/`watch` on `secrets`. Kubernetes
RBAC cannot scope `secrets` by label, so static analysis tools will still flag
broad Secret access; only enable when ExternalModels and credential Secrets
actually span namespaces.

Defense in depth on the workload:

- The Secret informer uses a **label selector** at list/watch time so only
  `ipp-managed` Secrets enter the cache (reduces blast radius if the process is
  compromised; it does not replace least-privilege RBAC). `Reconcile` still
  checks the label after `Get` before storing credentials.

# Vertex AI OpenAI-Compatible Provider (`vertex-openai`)

## Overview

The `vertex-openai` provider routes requests to Google Vertex AI's OpenAI-compatible
chat/completions endpoint. This is a pass-through translator — the request body is not
mutated since the endpoint accepts OpenAI format natively. The translator rewrites the
`:path` header to include the GCP project, location, and endpoint.

## Important: Plugin Configuration Required

Unlike other providers, `vertex-openai` requires plugin-level configuration with your
GCP project, location, and endpoint. These values are set in the Helm chart's plugin config:

```yaml
plugins:
  - type: api-translation
    name: api-translation
    json:
      vertexOpenAI:
        project: "<GCP_PROJECT_ID>"
        location: "us-central1"
        endpoint: "openapi"
```

Without this config, the `vertex-openai` translator will not be registered and requests
will fail with `unsupported provider - 'vertex-openai'`.

## Important: OAuth Token Expiry

Vertex AI uses OAuth2 tokens that **expire every hour**. Unlike other providers where
the API key is long-lived, the Vertex token in the K8s Secret must be refreshed manually:

```bash
# Generate a new token
gcloud auth print-access-token

# Update the Secret
kubectl create secret generic vertex-api-key -n llm \
  --from-literal=api-key="$(gcloud auth print-access-token)" \
  --dry-run=client -o yaml | kubectl apply -f -
```

## Configuration

| Field | Value |
|-------|-------|
| Provider type | `vertex-openai` |
| ExternalModel endpoint | `{region}-aiplatform.googleapis.com` (e.g., `us-central1-aiplatform.googleapis.com`) |
| Auth header | `Authorization: Bearer <OAUTH_TOKEN>` |
| API path | `/v1/projects/{project}/locations/{location}/endpoints/{endpoint}/chat/completions` |
| Request format | OpenAI Chat Completions (pass-through) |
| Response format | `usage.extra_properties` stripped; rest is OpenAI-compatible |
| Plugin config | Required: `project`, `location`, `endpoint` (see below) |

### Plugin Config Fields

| Field | Description | Example |
|-------|-------------|---------|
| `project` | GCP project ID | `my-gcp-project` |
| `location` | GCP region | `us-central1` |
| `endpoint` | Vertex AI endpoint ID — use `openapi` for the OpenAI-compatible endpoint | `openapi` |

## Response Field Stripping

Vertex adds `usage.extra_properties` (containing Google-specific metadata like `traffic_type`)
to responses. The translator strips this field automatically.

## ExternalModel Example

```yaml
apiVersion: maas.opendatahub.io/v1alpha1
kind: ExternalModel
metadata:
  name: my-vertex-model
  namespace: llm
spec:
  provider: vertex-openai
  targetModel: google/gemini-2.5-flash
  endpoint: us-central1-aiplatform.googleapis.com
  credentialRef:
    name: vertex-api-key
```

Note: The `targetModel` must use `publisher/model` format (e.g., `google/gemini-2.5-flash`).

## Secret Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vertex-api-key
  namespace: llm
  labels:
    inference.networking.k8s.io/bbr-managed: "true"
type: Opaque
stringData:
  api-key: "ya29.a0..."  # OAuth token from gcloud auth print-access-token
```

## How to Get Access

1. Create a GCP project at https://console.cloud.google.com
2. Enable Vertex AI API: https://console.cloud.google.com/apis/library/aiplatform.googleapis.com
3. Ensure your account has the `Vertex AI User` role (`roles/aiplatform.user`)
4. Install gcloud CLI: `brew install google-cloud-sdk`
5. Authenticate: `gcloud auth login`
6. Generate token: `gcloud auth print-access-token`

## Supported Models

Any model available on Vertex AI's OpenAI-compatible endpoint:
- `google/gemini-2.5-flash`, `google/gemini-2.5-pro`
- `google/gemini-2.0-flash`
- Third-party models deployed on Vertex AI

Use the `publisher/model` format for all model names.

## Troubleshooting

**`unsupported provider - 'vertex-openai'`:**
The plugin config is missing. Ensure the Helm values include `vertexOpenAI` config with
`project`, `location`, and `endpoint`.

**404 on inference:**
- Check that `targetModel` uses `publisher/model` format (e.g., `google/gemini-2.5-flash`,
  not just `gemini-2.5-flash`)
- Check that the `endpoint` in plugin config is `openapi` (not a custom endpoint name)
- Verify the API version — `v1beta` may return 404; the translator uses `v1`

**Empty response / timeout:**
The OAuth token in the Secret has likely expired. Refresh it with
`gcloud auth print-access-token` and update the Secret.

**`ExternalModel is invalid: spec.provider: Unsupported value: "vertex-openai"`:**
The ExternalModel CRD on your cluster doesn't include `vertex-openai` in the provider enum.
Update the CRD from the latest MaaS repo or patch it:
```bash
oc patch crd externalmodels.maas.opendatahub.io --type=json -p '[
  {"op":"add","path":"/spec/versions/0/schema/openAPIV3Schema/properties/spec/properties/provider/enum/-","value":"vertex-openai"}
]'
```

## Testing

```bash
# Direct API test (Vertex native OpenAI-compatible)
curl -sk "https://us-central1-aiplatform.googleapis.com/v1/projects/<PROJECT>/locations/us-central1/endpoints/openapi/chat/completions" \
  -H "Authorization: Bearer $(gcloud auth print-access-token)" \
  -H "Content-Type: application/json" \
  -d '{"model":"google/gemini-2.5-flash","messages":[{"role":"user","content":"Say hello"}],"max_tokens":100}'

# Through MaaS gateway
curl -sk "https://${GATEWAY_HOST}/llm/my-vertex-model/v1/chat/completions" \
  -H "Authorization: Bearer ${MAAS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"google/gemini-2.5-flash","messages":[{"role":"user","content":"Say hello"}],"max_tokens":100}'
```

## Official Documentation

- Vertex AI OpenAI-compatible API: https://cloud.google.com/vertex-ai/generative-ai/docs/multimodal/call-gemini-using-openai-library
- Chat Completions: https://cloud.google.com/vertex-ai/generative-ai/docs/reference/rest/v1/projects.locations.endpoints.chat/completions
- Models: https://cloud.google.com/vertex-ai/generative-ai/docs/learn/models
- Authentication: https://cloud.google.com/docs/authentication

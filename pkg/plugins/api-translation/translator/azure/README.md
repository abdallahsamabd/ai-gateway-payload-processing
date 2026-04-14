# Azure OpenAI Translator

## Overview

The Azure OpenAI translator handles request and response translation between the [OpenAI Chat Completions](https://platform.openai.com/docs/api-reference/chat) format used by the gateway and the [Azure OpenAI v1 Chat Completions API](https://learn.microsoft.com/en-us/azure/ai-services/openai/reference).

Azure OpenAI uses the same request/response schema as OpenAI, so translation is limited to **path rewriting** and **response cleanup** (stripping Azure-specific fields).

## API Endpoint

| Property | Value |
|----------|-------|
| Provider key | `azure-openai` |
| Target path | `POST {endpoint}/openai/v1/chat/completions` |
| API version | `v1` (default, no `api-version` query parameter required) |
| Content-Type | `application/json` |

The `{endpoint}` is the Azure OpenAI resource hostname, e.g. `https://{your-resource-name}.openai.azure.com`.

## Authentication

| Header | Format |
|--------|--------|
| `api-key` | Raw key value (no prefix) |

The `api-key` header is injected by the [api-key injection plugin](../../../apikey-injection/plugin.go), using the key stored in the Kubernetes Secret referenced by the `ExternalModel` CR.

### How to Create an API Key

#### Prerequisites

- An Azure account with an active subscription ([create one for free](https://azure.microsoft.com/free/))
- Access granted to Azure OpenAI in your subscription. New users must [request access](https://aka.ms/oai/access)

#### Steps

1. Sign in to the [Azure Portal](https://portal.azure.com/)
2. Click **Create a resource** and search for **Azure OpenAI**
3. Select **Azure OpenAI** and click **Create**
4. Fill in the required fields:
   - **Subscription**: Select your Azure subscription
   - **Resource group**: Create new or select existing
   - **Region**: Choose a region that supports the models you need (e.g. `East US`, `West Europe`)
   - **Name**: Choose a unique name — this becomes your endpoint hostname (`{name}.openai.azure.com`)
   - **Pricing tier**: Select `Standard S0`
5. Click **Review + create**, then **Create**
6. Once deployed, go to the resource and navigate to **Resource Management** > **Keys and Endpoint**
7. Copy **Key 1** or **Key 2** — either key works

#### Deploying a Model

After creating the resource, you need to deploy a model before you can make API calls:

1. Go to [Azure AI Foundry](https://ai.azure.com/)
2. Select your resource and navigate to **Deployments**
3. Click **Create deployment**
4. Choose a model (e.g. `gpt-4o`, `gpt-4o-mini`) and give it a deployment name
5. Use this deployment/model name as the `model` field in your API requests

For detailed instructions, see:
- [Create and deploy an Azure OpenAI resource](https://learn.microsoft.com/en-us/azure/ai-services/openai/how-to/create-resource)

## Supported Request Fields

All standard [OpenAI Chat Completions request fields](https://platform.openai.com/docs/api-reference/chat/create) are supported, including:

`messages`, `model`, `temperature`, `top_p`, `max_tokens`, `max_completion_tokens`, `stream`, `stop`, `n`, `presence_penalty`, `frequency_penalty`, `logit_bias`, `seed`, `tools`, `tool_choice`, `response_format`, `reasoning_effort`

For the full list of supported fields, see the [Azure OpenAI REST API reference](https://learn.microsoft.com/en-us/azure/ai-services/openai/reference).

## Official Documentation

- [Azure OpenAI REST API reference](https://learn.microsoft.com/en-us/azure/ai-services/openai/reference)
- [Azure OpenAI models](https://learn.microsoft.com/en-us/azure/ai-services/openai/concepts/models)

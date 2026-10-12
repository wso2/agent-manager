# Run the external insurance agent through an Amazon Bedrock gateway

This sample keeps the insurance tools and FastAPI contract. Set
`MODEL_PROVIDER=bedrock-gateway` to use the Strands Bedrock client with the
WSO2 AI gateway's native Converse route. The default remains OpenAI.

## Configure the provider

1. In Agent Manager, add an LLM service provider using the Amazon Bedrock
   template, the regional runtime endpoint, and an upstream Bedrock API key.
   Generate a short-term key in the Amazon Bedrock console under **API keys**.
   Keep it in the provider and renew it before expiry; never put it in the agent image.
2. Use context `/insurance-bedrock`, version `v1.0`, and upstream endpoint
   `https://bedrock-runtime.us-east-1.amazonaws.com`. The current console calls
   the template **AWS Bedrock**; the AWS service name is **Amazon Bedrock**.
   Its upstream Authorization header carries the Bedrock bearer key.
3. On the external agent, choose **Configure → Add LLM Configuration**, select
   this provider and save. In **Connect to LLM Provider**, copy the endpoint URL
   and one-time gateway key. The client header is `API-Key` (HTTP header names
   are case-insensitive). Verify deployment in the same environment.
4. Verify a request without the gateway key is rejected before exposing the
   route to callers.

## Configure the agent

```text
MODEL_PROVIDER=bedrock-gateway
BEDROCK_REGION=us-east-1
BEDROCK_MODEL_ID=amazon.nova-micro-v1:0
LLM_GATEWAY_URL=https://<gateway-host>/insurance-bedrock
LLM_GATEWAY_API_KEY=<injected gateway key>
COMPANY_NAME=AnyCompany Insurance
```

Keep `AMP_OTEL_ENDPOINT` and `AMP_AGENT_API_KEY` for the existing external-agent
registration. The model and telemetry keys serve different purposes. In ECS,
reference keys through Secrets Manager. Remove the OpenAI key from this task.

`agent.py` uses unsigned, non-streaming Converse requests with a dedicated
API-key header. It removes ambient AWS Authorization headers before sending.
The gateway authenticates the agent and applies the upstream Bedrock bearer
credential. The agent requires no AWS inference credential. By default,
responses are limited to 600 tokens and the model temperature is zero.

The Bedrock template does not imply OpenAI-to-Bedrock API translation. Use the
Bedrock adapter, rather than pointing `OpenAIModel` at this endpoint.

Confirm `amazon.nova-micro-v1:0` is available to your account in `us-east-1`,
with upstream `bedrock:InvokeModel` permission for non-streaming Converse.
A different model may require an inference profile and additional permissions.
Short-term keys inherit the generating principal's permissions and expire within
12 hours. Keep this demo's key refreshed in the provider.

## Add a prompt policy

On **Guardrails**, add **Prompt Decorator**. For the v1.0.2 policy schema:

```json
{
  "promptDecoratorConfig": {
    "text": "Only answer insurance support questions. Decline unrelated creative requests."
  },
  "jsonPath": "$.system[0].text",
  "append": false
}
```

Confirm the deployed gateway exposes this schema before saving; older versions
use different parameters. This targets native Converse system text, not an
OpenAI message array. Test a policy lookup and an unrelated request before and
after attachment with fresh sessions. Prompt decoration is guidance, not
guaranteed blocking. Agent spans can capture prompts before gateway modification;
use controlled gateway-side inspection to verify the decorated payload.
See the [policy definition](https://github.com/wso2/gateway-controllers/blob/634a791630a62b9d2afd4c17f4e210cbddf1125e/policies/prompt-decorator/policy-definition.yaml).

## Verify and clean up

Send a synthetic policy-list request to `/chat`, confirm the three fixture
policy IDs, and inspect the `list_policies` tool call in Agent Manager traces.
Test gateway policies separately: configuring the provider does not enable
Amazon Bedrock Guardrails or establish evaluation quality.

Stop the test task after validation. Revoke the dedicated gateway
key when no longer required. The upstream short-term token expires; this demo
has no automatic token refresh. Do not use it as unattended production setup.

## Regression test

Run the offline adapter regression from this directory after installing
the agent requirements:

```bash
PYTHONPATH=agent python tests/test_bedrock_gateway.py
```

This test uses a local mock Converse server with the real Strands client.
It checks the tool round trip, gateway key, exclusion of ambient AWS
credentials, unchanged OpenAI default, and rejection of missing configuration.
It makes no AWS model calls.

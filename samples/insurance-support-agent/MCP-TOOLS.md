# Connect the included insurance MCP server through AgentID

The sample includes a runnable Streamable HTTP MCP server and an OAuth-capable
Strands client. You deploy and configure them; no MCP implementation is required.
The server exposes `list_policies` and `lookup_policy` over the same synthetic
fixtures as the local agent. It has no claim-writing tools.

With `USE_MCP=true`, the agent uses only the two remote policy tools. It does not
retain local policy or claim tools as a fallback. OpenAI and the Bedrock gateway
model paths both support this mode. Leave `USE_MCP` unset for the original local
five-tool experience.

## 1. Deploy the supplied server

From `samples/insurance-support-agent`, build its container:

```sh
docker build --platform linux/amd64 -f mcp-server/Dockerfile -t insurance-mcp:latest .
```

Use [ECS-EXTERNAL.md](ECS-EXTERNAL.md) for the full ECS setup and task templates.
Push it to Amazon ECR using that guide. In the ECS
console, create a Fargate task definition using this image, port `8001`, a log
destination, and a task execution role that can pull the image and read its secret.
Generate a dedicated random key in your secret-management workflow, store it in
AWS Secrets Manager, and inject it as `INSURANCE_MCP_UPSTREAM_KEY`.

Run the task as an ECS service behind an HTTPS Application Load Balancer. Set
its target group's health check path to `/health` and target port to `8001`.
Use an ACM certificate for the HTTPS listener. Allow traffic to the task only
from the load balancer security group. The resulting
`https://<server-host>/mcp` endpoint must be reachable by the Agent Manager gateway.
The `/health` endpoint is public but returns only a static health status.
Every other HTTP request requires `X-Insurance-Upstream-Key`.

For local smoke tests, install `mcp-server/requirements.txt`, set the upstream key,
and run `python mcp-server/server.py`. This listens on port `8001`; local testing
does not replace a reachable HTTPS endpoint for the SaaS gateway.

## 2. Register and authorize the proxy in the console

1. In Agent Manager's organization view, open **Resources → MCP Proxies** and
   add the server's HTTPS `/mcp` endpoint. Under **Advanced Configurations**, set
   **Header** to `X-Insurance-Upstream-Key` and **Value** to the upstream key.
   Use proxy handle `insurance-policies`. Fetch tools and save the proxy. Keep this key in the gateway
   configuration; do not give it to the agent.
2. In **Manage Tools**, choose **Deny all**, then allow only `list_policies` and
   `lookup_policy`. Deploy the proxy to the environment used by your agent.
3. On **Security**, select **OAuth**. Create action `read`, producing scope
   `insurance-policies:read`, and map **both** policy tools to it. Under
   **Agent ID → Roles**, create `policy-reader` in the same environment, add
   that scope, and assign the external agent's identity. Apply the configuration.
   Unscoped tools default to permitted for authenticated callers: map every tool
   you intend to restrict. The [authorization guide](https://wso2.com/agent-platform/docs/cloud/guides/authorize-agent-access-to-mcp-tools/)
   provides scope/role/assignment API requests if your console differs.
4. On the external agent, open **Configure → Tool Configurations → Add Tool
   Configuration**, select the insurance proxy, and save. Open **Connect to MCP
   Server** for the target environment and copy the proxy endpoint. Open the
   agent's **Agent ID → Overview** in that environment. Copy the OAuth client ID,
   choose **Regenerate Secret**, and store the secret. Copy **Token Endpoint**
   under **OAuth2 Endpoints**. The overview card's Agent ID is not the client ID.

## 3. Configure the supplied agent client

In the ECS task definition for the **agent**, add:

```text
USE_MCP=true
INSURANCE_MCP_URL=<exact proxy endpoint from Connect to MCP Server>
AMP_AGENTID_CLIENT_ID=<client ID>
AMP_AGENTID_TOKEN_ENDPOINT=<token endpoint>
AMP_AGENTID_SCOPES=insurance-policies:read
```

Inject `AMP_AGENTID_CLIENT_SECRET` from Secrets Manager, then deploy a new task
revision. Preserve the existing model and telemetry settings. The proxy URL is
not the upstream server URL. Do not add the upstream key to the agent container.

The supplied client uses `client_credentials` with `client_secret_basic`, the
configured scope, and the proxy URL as the OAuth `resource`. It caches the bearer
token and renews it before expiry. HTTPS is required except for loopback test
endpoints; redirects are not followed. At startup, the agent connects to the
proxy and verifies that both tools are available. Missing configuration or a
failed connection prevents startup. The MCP connection closes on shutdown.

## 4. Verify allowed and denied access

1. Send `List my insurance policies using your tools` to the agent's `/chat`
   endpoint using a new session ID. Confirm the fixture policies and inspect
   the remote `list_policies` tool call in its Agent Manager trace.
2. Ask for `OZ-HOME-2280` and confirm that `lookup_policy` returns the fixture.
3. Remove the agent from `policy-reader` and restart the agent task to discard
   its cached token and conversation history. Confirm that token acquisition or
   MCP tool discovery is denied. The agent must not fall back to local tools.
   Previously issued tokens can remain valid until expiry.
4. Restore the role and restart the task to resume the walkthrough. An HTTP
   authorization failure can close the MCP transport; restart after correcting
   credentials or policy rather than retrying through a different route.

The server authenticates the gateway using its upstream key. AgentID scope
checks happen at the proxy. This fixture example is not customer-level policy
isolation; production tools need their own subject/tenant authorization.
AWS IAM governs AWS resource access separately.

## Test and clean up

After installing `agent/requirements.txt`, run from the sample directory:

```sh
PYTHONPATH=agent python tests/test_mcp.py
PYTHONPATH=agent python tests/test_bedrock_gateway.py
```

The MCP tests use the real server and client with a mock OAuth endpoint and
proxy, including a model-to-MCP tool round trip. They do not validate a live
Agent Manager role policy.

Remove the test service (or scale it to zero), load balancer and target group,
test-only images/logs/secrets, proxy binding, scope, role, and proxy when no
longer needed. Revoke the dedicated credentials and preserve shared resources.

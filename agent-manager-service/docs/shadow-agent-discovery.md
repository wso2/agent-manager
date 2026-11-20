# Shadow Agent Discovery

Shadow Agent Discovery finds the agents your organization runs outside Agent Manager: the **unregistered agents**. It finds them in two places.

- **AI platforms** (OpenAI, Anthropic, Amazon Bedrock, Google Agent Platform, Microsoft Foundry). Agent Manager connects to each platform with a read-only credential and calls only the platform's agent listing APIs. Every agent it shows is an agent object the platform itself lists, with the evidence of which listing returned it; usage, logs and model calls are not read, so nothing is inferred or scored.
- **Source code repositories** (GitHub). Agents that exist only in code are found by a scan command the customer runs on their own machine (see [Source code scans](#source-code-scans)). Agent Manager never stores a Git token and never pulls code: the command publishes only the agents it confirms.

## Access

| What | Read | Change |
|---|---|---|
| Platforms catalog, connections, scans, unregistered agents | `amp:shadow-agent:read` | — |
| Add platforms, test, run and clear scans, create scan commands, onboard and delete agents | — | `amp:shadow-agent:manage` |
| Publish the results of a source code scan | — | `amp:shadow-agent:publish` |

All three are granted to the Agent Manager Admin role only: connections hold platform credentials. `amp:shadow-agent:publish` is held by the scan token and used by the scan command only: the publish endpoint refuses a user token, even an Admin's. Every change is in the audit trail (`shadow-connection:*` and `shadow-scan-token:issue` are credential operations; `shadow-scan:create`, `shadow-scan:publish`, `shadow-scan:clear`, `shadow-finding:onboard`, `shadow-finding:hide`, `shadow-finding:delete`, `shadow-finding:clear`). `shadow-scan-token:issue` and `shadow-scan:publish` are fail-closed: nothing is issued or saved unrecorded.

## Platforms

The API calls a platform a *provider* (`provider` fields, the `providers` route). The **Platforms** tab and **Add a platform** show them in three sections: **AI platforms**, **Source code repositories** and **Custom platforms** (your own agent inventory API). Azure DevOps is no longer a platform: migration 067 deletes any Azure DevOps platform with its agents and evidence.

| Platform | Credential | What a scan lists |
|---|---|---|
| OpenAI | A read-only project key per project (scope `api.agents.read` or `api.all.read`) | `GET /v1/agents` of each project with a key |
| Anthropic | A key per workspace | `GET /v1/agents?include_archived=true` (Managed Agents) of each workspace with a key |
| Amazon Bedrock | Access key, optionally assuming a cross-account read-only role | Per region: AgentCore runtimes (`ListAgentRuntimes`, `GetAgentRuntime`), AWS Agent Registry agent records (`ListRegistries`, `ListRegistryRecords`) and classic Bedrock agents (`ListAgents`, `GetAgent`, `ListAgentActionGroups`) |
| Google Agent Platform | Service account key | Agent Runtime engines (`reasoningEngines.list`) and Agent Registry agents (`agents.list`) in each project and location |
| Microsoft Foundry | Entra app with the custom role shown in the console | The Foundry projects of each AI Services account and their agents (`GET {projectEndpoint}/agents`), and optionally classic agents (`GET {projectEndpoint}/assistants`) |
| Custom platform | An API key for the customer's own agent inventory API, sent as `Authorization: Bearer <API key>` | `GET {Agents URL}?limit=100`, following `nextCursor`: the agents the customer's service returns (see **Custom platform** below) |
| GitHub (source code repositories) | None stored. For private repositories, the scan command asks for a fine-grained personal access token on the customer's machine: Contents read-only (GitHub adds Metadata read-only). A classic token is not suitable: its `repo` scope also grants write access | Stored: the organization or user account, every repository or a list of them, whether to include private repositories, whether to read CI pipeline definitions (GitHub Actions workflows), and optionally a self-hosted server's API URL (GitHub Enterprise Server or GitHub Enterprise Cloud with data residency, GHE.com) |

What is left out, because it is not an agent: an AgentCore runtime whose `serverProtocol` is `MCP` (an MCP server), AWS Agent Registry records of type MCP or gateway (and a record about an MCP runtime), and Google's built-in registry agents.

**OpenAI and Anthropic keys.** Agents are listed per project (OpenAI) or workspace (Anthropic), with that project's or workspace's own key, so a connection needs at least one. There is no Admin API key: it cannot call `GET /v1/agents`, and no other endpoint is called. An Admin key stored by an earlier version is not used and is dropped when the platform is next saved.

The console shows, for each AI platform, what Agent Manager does with each credential, the least privilege to grant (with ready-to-use role and policy documents), the official endpoints, and which evidence each listing provides (**Test connection**). A source code repository has no credential to test: the scan command checks the token when it runs.

**Endpoints.** Each AI platform connection stores the base URL of every endpoint it calls, prefilled with the official URL or URL pattern (`{region}`, `{location}`). An endpoint can be changed, for example to reach the platform through a proxy; the console warns when a host is not an official one, because the stored credential is sent to it. The HTTP client never follows a redirect to another host. A source code repository's server URL is under the collapsed **Self-hosted server** section and has no official-host warning, because a self-hosted server can be on any host.

**Coverage options** (checkboxes, on by default): every Azure subscription the app can read; the Google `global` Agent Registry location and `us`/`eu` multi-regions; Foundry classic agents.

**Custom platform.** For agents that only the customer's own systems know about. The customer implements one call, `GET` on an **Agents URL** they choose, to the OpenAPI contract the form offers for download (`Download contract`; served as `apiContract` with the provider, and kept in `services/shadowagents/custom_platform_openapi.yaml`). Agent Manager sends the API key as a bearer token, adds only `limit` (100 in a scan, 1 in **Test connection**) and `cursor` (the previous page's `nextCursor`), and reads at most 100 pages. Each agent needs an `id` (at most 256 characters) and a `name`; `description`, `source`, `url`, `framework`, `model`, `version`, `status`, `scope`, `owner` (`name`, `email`), `createdAt` and `labels` are optional. Any other field is ignored and never stored; an agent without an `id` or `name`, or that does not match the contract, is left out and counted in the scan result, and a repeated `id` is listed once. `url` is kept without its query string, fragment and user info. The Agents URL must be http(s), without a query string, fragment or user info; the server must be reachable from Agent Manager. The API key is stored encrypted (as `customApiKey`, apart from the `apiKey` an earlier version used for OpenAI Admin keys). **See what it lists** gives the setup steps.

**Amazon Bedrock form.** Session token, Role to assume and External ID are behind **Advanced configuration** (it opens by itself when one of them is set); the AWS STS endpoint is shown under Role to assume. Under **Services to scan**, each service's endpoint is shown right under its checkbox while it is ticked.

**Services to scan (Amazon Bedrock, Google Agent Platform).** Each listing a connector can skip is a checkbox, on by default, stored as `config.disabledServices` (an absent key means the service is scanned). Bedrock: AgentCore runtimes, AWS Agent Registry, classic Bedrock agents. Google: Agent Registry (the Agent Runtime engines are always listed). An unticked service is never called: its endpoint field is hidden, its calls are listed as *skipped (not selected in this connection)*, **Test connection** does not probe it, and the capabilities that need it are dropped. A Bedrock connection needs at least one service. The AWS STS endpoint is shown only when a role to assume is entered, because STS is called only to assume it. Grant the least privilege of the selected services only.

## Scans

**Run scan** scans every AI platform, or the platforms you choose in its dialog; **Scan** on a platform's row in **Platforms** scans that platform only (`POST .../scans`, with `connectionIds` for a choice). A scan lists each connection's agents in the background. One scan per organization can be queued or running at a time; scans are leased in the database, so any service replica can run them and a scan whose worker stopped is picked up again. Each connection runs on its own: a platform that fails, refuses a permission or rate limits fails only its own calls, and the scan result lists the outcome of every call. Retries honour `Retry-After`. GitHub connections are never part of these scans: naming one in `connectionIds` is refused, and they are scanned by their scan command instead.

Each scan records the agents it found, so **Scans** > **View agents** lists one scan's agents (`GET .../findings?scanId=`).

**Scans** > **Clear scans** (`DELETE .../scans`) deletes the finished scans and their per-scan agent lists. A queued, running or pending scan is kept, and the agents and their evidence are not changed.

## Source code scans

**1. The platform form.** On the add or edit page of a GitHub platform, enter the organization and the repositories, one per line, or tick **Scan every repository** (unticked at first). There is also **Include private repositories**. **Also read CI pipeline definitions** reads pipeline definitions only. A GitHub repository can be given as `owner/name` of the connection's owner; it is stored as the name. **How it works**, next to the platform name, opens the whole flow and the token guidance in a dialog.

**2. The Scan command panel.** Next to the form, the panel builds the command as you type, with two switches above it:

- **AI review**: the customer's LLM checks the matches the rules cannot prove. The command asks for an LLM API key.
- **LLM judge**: a second, smaller model must agree before an agent is published. Needs AI review.

The choices are stored with the scan (`options`). Without AI review, only the agents the rules prove are published.

**3. The command.** **Add platform and get command** (or **Save and get command** when editing) saves the platform and creates the scan (`POST .../connections/{connectionId}/runner-scans`). This creates a `pending` scan and a scan token valid for 1 hour, and the console fills the scan ID, token, Agent Manager address and image into the command. The command is shown once: the form locks, and **Done** leaves the page. The token is kept only in the page's memory, never in a URL, storage or the query cache. For a saved platform, **Generate command** on its row in **Platforms** does the same in a dialog. The command is available only where `AMP_SHADOW_RUNNER_ENABLED` is on (see [Configuration](#configuration)); otherwise the platform is saved and getting the command answers 503.

The command has one option per line; values you entered are shell-quoted:

```bash
docker run --rm -it \
  --user "$(id -u):$(id -g)" \
  -v "$PWD:/out" \
  -e AMP_URL=<amp-url> \
  -e AMP_SCAN_TOKEN=<scan-token> \
  <scanner-image> scan \
  --scan-id <scan-id> \
  --github-org <organization> \
  --repos=<repositories> \
  --include-private \
  --read-ci \
  --ai-review \
  --judge
```

| Platform | Options |
|---|---|
| GitHub | `--github-org <organization>`; `--all-repos` or `--repos=a,b`; `--include-private`; `--read-ci`; `--github-api-url <url>` only when it is not the default |
| Any | `--ai-review` with AI review; `--judge` with the LLM judge |

The scan command's image (`AMP_SHADOW_RUNNER_IMAGE`) is not released yet, so the command cannot be run until it is.

**4. On the customer's machine.** The command asks for the Git token (for GitHub, only when private repositories are included) and for the LLM API key (only with AI review). It lists the repositories it will scan and asks to confirm, scans them, deletes the code, and publishes only the confirmed agents, with the names of the repositories it scanned and could not scan. The agents appear in **Unregistered Agents** as code agents.

**Pending scans.** The scan worker never runs a pending scan, and it does not count as the organization's queued or running scan. A pending scan ends when its command publishes. One with no results within 1 hour becomes failed with *expired: no results within 1 hour*. **Clear scans** keeps pending scans.

**Publish rules.**

- A publish is refused (409) when the scan was already published, has expired, or a newer scan of the same platform has already published.
- The scan becomes **succeeded**, **partial** when some repositories failed or agents could not be marked as no longer present (see below), or **failed** when every repository failed.
- An agent the command did not publish is marked as no longer present only in repositories the scan finished. The repositories it could not scan keep their agents and evidence. For a GitHub list of repositories, a configured repository the command did not report as scanned is kept too.
- Nothing is marked as no longer present when the command could not list every repository. New agents are still saved.
- Repository names are normalised like an agent's scope: cut to 256 characters, and for GitHub the `organization/` prefix is dropped and the name is lowercased.

The scan's API response (`GET .../scans/{scanId}`) lists the scanned and failed repositories, with each failure's reason.

## Unregistered agents

Each agent is keyed by its platform, its kind and the platform's own ID of it (an agent ID, a runtime ARN, an engine resource name, or the scan command's ID of a code agent). The kinds are **runtime** (an AgentCore runtime or a Google Agent Runtime engine), **platform agent** (any other agent object a platform lists) and **code agent** (`code_agent`, shown as *Agent in code*: an agent a source code scan found in a repository). A source code repository keeps only code agents, and an AI platform only runtimes and platform agents. Evidence about the same agent from two listings is one agent: an AWS Agent Registry record whose provenance names an AgentCore runtime is merged into the runtime, and a Google Agent Registry entry for an engine is merged into the engine.

**No duplicates.** The **Unregistered Agents** tab (`GET .../findings`) shows each agent once, however many connections found it (two connections to the same OpenAI organization, say): the row is the agent as the connection that saw it last reports it, and **Source** lists every connection. The list shows the most recently seen agents first. The summary counts each agent once too. Filter by platform or onboarding at the top right, or search by name, platform ID or scope.

**Onboarding as an external agent.** **Onboard as external agent** on an agent's page shows what onboarding does and the steps that follow, then registers the agent as an externally-hosted agent in a project (the same request as the console's Register an agent page) and links the unregistered agent to it (`POST .../findings/{findingId}/onboarding` with `projectName` and `agentName`; the registered agent must exist). The link is the agent's: it holds for every connection that found it, a connection that finds it later inherits it, and an agent is onboarded once. The **Onboarding status** column shows Onboarded or Not onboarded, filter with `onboarded=true|false` (the tab shows Not onboarded by default), and the agent's page links to the registered agent. The workload keeps running where it is; set it up with `amp-instrument` and route its model calls through an organization LLM provider from the registered agent's pages.

**Removing an onboarded agent from the list.** Once an agent is onboarded, the console asks whether to remove it from the Unregistered Agents list or keep it there marked as Onboarded; the agent's page can change the choice later (**Remove from list** / **Show in list**). `PUT .../findings/{findingId}/hidden` with `{"hidden": true|false}` sets it on every connection's finding of the agent; only an onboarded agent can be removed (400 otherwise). A removed agent stays out of the list after later scans, and a connection that finds it later inherits the choice; it can still be opened by its ID, and the summary still counts it as onboarded. Deleting is different: a later scan lists a deleted agent again, not onboarded.

**Deleting agents.** Each row has a delete button (`DELETE .../findings/{findingId}`): it deletes the agent and its evidence for every connection that found it. **Delete all**, at the top right (`DELETE .../findings`), deletes every agent of the organization, whatever the filters show, including the agents their platforms no longer list; it returns how many agents it deleted. Platforms and scan history are kept, and nothing changes on the platforms: a later scan lists an agent again if a platform still has it.

**Only agents that exist.** When every listing of a connection succeeds completely, an agent of that connection the scan did not return is marked as no longer listed (`present: false`): it was deleted at the platform, or is no longer in the connection's scope (a removed project key, region or service). The tab shows only agents that are still listed. A scan with a failed or incomplete listing (an error, a refused permission, a page limit) marks nothing, so a platform error never hides an agent. For source code scans, see the publish rules above.

**Evidence.** Each agent shows the listing that returned it, the exact condition in the platform's field names, and what was observed (such as a runtime's status). A Foundry *external* agent is also flagged as a *possible duplicate* of an agent from another platform with the same name (Foundry stores no endpoint for external agents, so they are never merged). Code agents are not matched by name against Foundry external agents.

Each piece of a code agent's evidence has one of twelve codes, `github.d1_agent_dependency` to `github.d12_developer_tooling`, with the file it was found in:

| Code | Evidence |
|---|---|
| `d1_agent_dependency` | A manifest or lockfile declares an agent framework (LangChain or LangGraph, CrewAI, AutoGen, Semantic Kernel, OpenAI Agents SDK, Google ADK, Spring AI) |
| `d2_agent_import` | A source file imports an agent framework |
| `d3_mcp_client_config` | An `.mcp.json` or `mcp.json` file, or an MCP client section in a Spring `application.yml` |
| `d4_mcp_client_code` | A source file creates an MCP client |
| `d5_a2a_artifact` | A `.well-known/agent-card.json` file, or a dependency on or import of the A2A SDK |
| `d6_agent_platform_iac` | Terraform, CloudFormation or Bicep declares an agent platform resource |
| `d7_tool_calling` | A source file passes tool definitions to a model SDK call |
| `d8_mcp_server_code` | A source file creates an MCP server |
| `d9_agent_definition` | A `*.prompty` or `agent.yaml` file |
| `d10_model_sdk` | A manifest or lockfile declares a model SDK and no agent framework |
| `d11_llm_env_var` | A model provider variable name (never its value) in a CI pipeline definition |
| `d12_developer_tooling` | A `.vscode/mcp.json`, `.cursor/mcp.json`, `AGENTS.md` or `CLAUDE.md` file; context only, not an agent on its own |

These are evidence codes only: an agent is published because the rules proved it or AI review confirmed it, never by a score.

**Details.** An agent shows what its listing reports, through an allow-list: model, framework, kind, version, status, tools and MCP servers (type and name; server label and URL origin), skills, image, source repository, region or location, project or workspace, execution role, protocol, labels and the platform's short description. Owner details are shown only when the agent listing reports who created the agent. A code agent also has the `reviewedBy` attribute in the API: who confirmed it (`rules`, `ai_review` or `ai_review_judge`). It can also have what deploying it needs: `branch` (the default branch scanned), `projectPath` (its project folder from the repository root, such as `/` or `/samples/support-agent`) and `language` (`python`, `typescript`, `javascript`, `csharp` or `ballerina`). The console shows it as **Confirmed by**: Rules, AI review, or AI review and LLM judge. With AI review, the agent detail page notes that AI review can make mistakes.

## Data retention

| Data | Kept |
|---|---|
| Platform secrets | Never in plain text: stored AES-GCM encrypted with `ENCRYPTION_KEY`, never returned, logged or audited; the API shows only a redacted hint. A source code repository stores none |
| Git tokens, LLM API keys, source code | Never sent to or stored by Agent Manager: the scan command asks for the keys on the customer's machine. The code is never sent to Agent Manager; with AI review, it goes only to the customer's own LLM |
| Scan tokens | Never stored or audited: returned once, to the console |
| Instructions, system prompts, tool schemas and definitions, environment variable values, MCP headers, descriptions marked sensitive | Never decoded, stored or logged |
| Agent details | The allow-listed attributes above; MCP and agent URLs reduced to scheme and host; a repository URL to scheme, host and path |
| Evidence | Evidence code, source (for a code agent, the file path) and observed flags, counts and short labels |
| Source code scans | The names of the scanned and failed repositories, and each failure's reason |

Error messages and failure reasons are scrubbed of anything that looks like a credential.

## API

All under `/api/v1/orgs/{orgName}/shadow-agents` (see `docs/api_v1_openapi.yaml`, tag *Shadow Agent Discovery*): `providers`, `summary`, `connections` (+ `/{id}`, `/{id}/test`, `/{id}/runner-scans`), `scans` (+ `/{id}`; `DELETE` clears the finished scans), `findings` (+ `/{id}`, `/{id}/onboarding`, `/{id}/hidden`).

**`POST .../connections/{connectionId}/runner-scans`** (`amp:shadow-agent:manage`) creates a pending scan of a GitHub connection and its scan token. Request: `{"aiReview": true, "llmJudge": false}` (the LLM judge needs AI review). Response (201, `Cache-Control: no-store`): `{scanId, token, expiresAt, ampUrl, image}`, the parts the console builds the command from. 400 for any other platform or the LLM judge without AI review; 404 for an unknown connection; 503 when source code scans are turned off or not set up (the message says what to set), when the identity provider is unavailable, or when the audit trail is unavailable.

**`POST /api/v1/shadow-agent-publisher/scans/{scanId}/results`** is the scan command's publish endpoint. Like the build publisher, it is not in the OpenAPI spec. It admits only a scan token: one this service obtained with its own client (`IDP_CLIENT_ID`) for one organization, holding `amp:shadow-agent:publish` and no other scope. A user token is refused (403), whatever its role. The organization comes from the token, never from the request.

```json
{
  "complete": true,
  "scannedRepositories": ["support-bot"],
  "failedRepositories": [{ "name": "legacy-app", "reason": "clone failed" }],
  "agents": [{
    "externalId": "support-bot:src/agent.py:SupportAgent",
    "name": "SupportAgent",
    "repository": "support-bot",
    "attributes": { "framework": "langgraph", "reviewedBy": "rules", "branch": "main", "projectPath": "/", "language": "python" },
    "evidence": [{ "code": "github.d2_agent_import", "source": "src/agent.py", "observed": { "imports": 1 } }]
  }]
}
```

- `complete`: the command could list every repository the connection's settings name.
- Repository names are GitHub repository names.
- Limits: a body of at most 8 MiB (413 above it); at most 1000 agents, 50 pieces of evidence per agent and 5000 names in each repository list (400 above them).
- The whole publish is rejected (400) when one agent is invalid: no `externalId`, an `externalId` repeated, no `repository`, no evidence, an evidence code that is not one of the connection's platform's codes, or `attributes.reviewedBy` missing or not allowed (`ai_review` needs a scan created with AI review, `ai_review_judge` one created with the LLM judge). Attributes outside the allow-list are dropped, and so is an attribute of the wrong shape: `tools` is a list of `{type, name}`, `mcpServers` a list of `{label, origin}`, `skills` and `protocolType` lists of strings, `labels` an object of strings, and every other attribute a single value. `branch`, `projectPath` and `language` are kept only when valid: `branch` at most 255 characters of `A-Z a-z 0-9 . _ / -`, with no `..`, no leading `-`, and no empty or dot-led part; `projectPath` `/` or a path of at most 1024 such characters that starts with `/`, with no `.`, `..` or empty segment and no trailing `/`; `language` one of `python`, `typescript`, `javascript`, `csharp` and `ballerina`. Unknown request fields are ignored.

Response (200): `{"scanId": "…", "status": "partial", "findingsCount": 1}`, the scan's new status and the number of agents saved. 404 when the scan is not a source code scan of the token's organization or source code scans are turned off; 409 under the publish rules above; 503 when the audit trail is unavailable.

## Configuration

Source code scans need the settings below. While `AMP_SHADOW_RUNNER_ENABLED` is off, or without `SERVER_PUBLIC_URL` or the identity provider client, `runner-scans` answers 503 and the rest of Shadow Agent Discovery works.

| Setting | Helm value (`agentManagerService.config.`) | Use |
|---|---|---|
| `SERVER_PUBLIC_URL` | `serverPublicURL` | Required. The Agent Manager address in the command (`AMP_URL`) |
| `IDP_TOKEN_URL`, `IDP_CLIENT_ID`, `IDP_CLIENT_SECRET` | `oidc.tokenUrl`, `oidc.clientId`, `oidc.clientSecret` or `oidc.existingSecret` | Required. The client the scan token is obtained with, as for the build token. The identity provider must grant `amp:shadow-agent:publish` to the client (`amp-api-client`; the bundled Thunder chart does) |
| `IDP_ORG_TOKEN_ENDPOINT_ENABLED` | `oidc.orgTokenEndpointEnabled` (chart default `false`) | Where the scan token (and the build token) is requested. Default `true`: from the organization token endpoint (`IDP_TOKEN_URL` with `/ou/<ouId>` inserted before `/oauth2/token`), for multi-org deployments such as WSO2 Cloud; the identity provider must serve it. The on-prem Helm chart sets `false`, since it runs a single organization and Thunder 1.x has no such endpoint: tokens then come from `IDP_TOKEN_URL` itself and name the client's own organization only |
| `AMP_SHADOW_RUNNER_ENABLED` | `shadowRunner.enabled` | Turns on scan commands and result publishing. Default `false`: while off, Generate command (`runner-scans`) answers 503 and publishing answers 404. The scan token is issued to `IDP_CLIENT_ID` and shown to the customer, so turn it on only where that client may issue a customer-visible token, such as WSO2 Cloud |
| `AMP_SHADOW_RUNNER_IMAGE` | `shadowRunner.image` | The command's image. Default `ghcr.io/wso2/amp-shadow-runner:latest` |
| `AMP_SHADOW_RUNNER_RESOURCE` | `shadowRunner.resource` | Optional RFC 8707 resource indicator the token is requested for. It becomes the token's audience, so `KEY_MANAGER_AUDIENCE` (`keyManager.audience`) must admit it; startup checks. Empty requests none, and the audience is `IDP_CLIENT_ID` |

## Limits

- An agent is found only where the connection can list it: an OpenAI project or Anthropic workspace without a key, a region or location not configured, or an unticked service is not listed.
- Agent Manager cannot tell which OpenAI projects or Anthropic workspaces have no key: add a key for each one to scan.
- Some platform fields (such as the AWS Agent Registry provenance and the Google `RuntimeReference`) are not documented exhaustively; connectors parse them leniently and record a call as `unsupported_response` when a response no longer has the documented shape.
- A code agent is found only in the repositories the scan command scanned, and only when it runs: Agent Manager does not scan repositories on its own.
- AI review and the LLM judge can make mistakes.
- A scan token is valid for 1 hour, and each scan accepts one publish: run each command once, and get a new one with **Generate command**.

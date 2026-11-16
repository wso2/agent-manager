# Shadow Agent Discovery

Shadow Agent Discovery finds the agents your organization runs on AI platforms outside Agent Manager: the **unregistered agents**. It connects to each platform with a read-only credential and calls only the platform's agent listing APIs. Every agent it shows is an agent object the platform itself lists, with the evidence of which listing returned it; usage, logs and model calls are not read, so nothing is inferred or scored.

## Access

| What | Read | Change |
|---|---|---|
| Platforms catalog, connections, scans, unregistered agents | `amp:shadow-agent:read` | — |
| Add platforms, test, run and clear scans, review agents | — | `amp:shadow-agent:manage` |

Both are granted to the Agent Manager Admin role only: connections hold platform credentials. Every change is in the audit trail (`shadow-connection:*` are credential operations; `shadow-scan:create`, `shadow-scan:clear`, `shadow-finding:update`, `shadow-finding:delete`, `shadow-finding:clear`).

## Platforms

The API calls a platform a *provider* (`provider` fields, the `providers` route).

| Platform | Credential | What a scan lists |
|---|---|---|
| OpenAI | A read-only project key per project (scope `api.agents.read` or `api.all.read`) | `GET /v1/agents` of each project with a key |
| Anthropic | A key per workspace | `GET /v1/agents?include_archived=true` (Managed Agents) of each workspace with a key |
| Amazon Bedrock | Access key, optionally assuming a cross-account read-only role | Per region: AgentCore runtimes (`ListAgentRuntimes`, `GetAgentRuntime`), AWS Agent Registry agent records (`ListRegistries`, `ListRegistryRecords`) and classic Bedrock agents (`ListAgents`, `GetAgent`, `ListAgentActionGroups`) |
| Google Agent Platform | Service account key | Agent Runtime engines (`reasoningEngines.list`) and Agent Registry agents (`agents.list`) in each project and location |
| Microsoft Foundry | Entra app with the custom role shown in the console | The Foundry projects of each AI Services account and their agents (`GET {projectEndpoint}/agents`), and optionally classic agents (`GET {projectEndpoint}/assistants`) |
| GitHub (source repositories) | Fine-grained personal access token: Contents read-only (Metadata read-only is added by GitHub) | Connection test only: the owner, the repositories the token reads, and the contents of one repository. Source code analysis is not available yet |
| Azure DevOps (source repositories) | Personal access token with Code (read) and Project and team (read), or a Microsoft Entra service principal with Basic access and project Readers | Connection test only: the projects, their Azure Repos repositories and the top-level items of one repository. Source code analysis is not available yet |

What is left out, because it is not an agent: an AgentCore runtime whose `serverProtocol` is `MCP` (an MCP server), AWS Agent Registry records of type MCP or gateway (and a record about an MCP runtime), and Google's built-in registry agents.

**OpenAI and Anthropic keys.** Agents are listed per project (OpenAI) or workspace (Anthropic), with that project's or workspace's own key, so a connection needs at least one. There is no Admin API key: it cannot call `GET /v1/agents`, and no other endpoint is called. An Admin key stored by an earlier version is not used and is dropped when the platform is next saved.

The console shows, for each platform, what Agent Manager does with each credential, the least privilege to grant (with ready-to-use role and policy documents), the official endpoints, and which evidence each listing provides (**Test connection**).

**Endpoints.** Each connection stores the base URL of every endpoint it calls, prefilled with the official URL or URL pattern (`{region}`, `{location}`). An endpoint can be changed, for example to reach the platform through a proxy; the console warns when a host is not an official one, because the stored credential is sent to it. The HTTP client never follows a redirect to another host.

**Coverage options** (checkboxes, on by default): every Azure subscription the app can read; the Google `global` Agent Registry location and `us`/`eu` multi-regions; Foundry classic agents.

**Amazon Bedrock form.** Session token, Role to assume and External ID are behind **Advanced configuration** (it opens by itself when one of them is set); the AWS STS endpoint is shown under Role to assume. Under **Services to scan**, each service's endpoint is shown right under its checkbox while it is ticked.

**Services to scan (Amazon Bedrock, Google Agent Platform).** Each listing a connector can skip is a checkbox, on by default, stored as `config.disabledServices` (an absent key means the service is scanned). Bedrock: AgentCore runtimes, AWS Agent Registry, classic Bedrock agents. Google: Agent Registry (the Agent Runtime engines are always listed). An unticked service is never called: its endpoint field is hidden, its calls are listed as *skipped (not selected in this connection)*, **Test connection** does not probe it, and the capabilities that need it are dropped. A Bedrock connection needs at least one service. The AWS STS endpoint is shown only when a role to assume is entered, because STS is called only to assume it. Grant the least privilege of the selected services only.

## Scans

**Run scan** scans every platform, or the platforms you choose in its dialog; **Scan** on a platform's row in **Platforms** scans that platform only (`POST .../scans`, with `connectionIds` for a choice). A scan lists each connection's agents in the background. One scan per organization can be queued or running at a time; scans are leased in the database, so any service replica can run them and a scan whose worker stopped is picked up again. Each connection runs on its own: a platform that fails, refuses a permission or rate limits fails only its own calls, and the scan result lists the outcome of every call. Retries honour `Retry-After`.

Each scan records the agents it found, so **Scans** > **View agents** lists one scan's agents (`GET .../findings?scanId=`). GitHub and Azure DevOps connections are part of a scan and recorded as *Source code analysis: skipped (not available yet)*; until that analysis exists, a scan's page shows labelled sample data for them, which is never stored.

**Scans** > **Clear scans** (`DELETE .../scans`) deletes the finished scans and their per-scan agent lists. A queued or running scan is kept, and the agents and their evidence are not changed.

**Source repository hosts (GitHub, Azure DevOps).** The connection form takes the organization (or GitHub user account), every repository/project the credential reads or a list of them, and whether CI pipeline definitions and `.env` example files will be read. Azure DevOps signs in with a personal access token or, when **Sign in with a Microsoft Entra service principal** is ticked, with a tenant ID, client ID and client secret; the credential of the other method is not kept. **Test connection** is real and read-only (GitHub REST API 2022-11-28; Azure DevOps REST 7.1, with `X-TFS-FedAuthRedirect: Suppress` so a refused credential is a 401 rather than the sign-in page).

## Unregistered agents

Each agent is keyed by its platform, its kind and the platform's own ID of it (an agent ID, a runtime ARN, an engine resource name). The kinds are **runtime** (an AgentCore runtime or a Google Agent Runtime engine) and **platform agent** (any other agent object a platform lists). Evidence about the same agent from two listings is one agent: an AWS Agent Registry record whose provenance names an AgentCore runtime is merged into the runtime, and a Google Agent Registry entry for an engine is merged into the engine.

**No duplicates.** The **Unregistered Agents** tab (`GET .../findings`) shows each agent once, however many connections found it (two connections to the same OpenAI organization, say): the row is the agent as the connection that saw it last reports it, and **Found by** lists every connection. The review status (new, reviewed, dismissed) is the agent's, so changing it changes it for every connection. The summary counts each agent once too. Filter by platform at the top right, or search by name, platform ID or scope.

**Deleting agents.** Each row has a delete button (`DELETE .../findings/{findingId}`): it deletes the agent and its evidence for every connection that found it. **Delete all**, at the top right (`DELETE .../findings`), deletes every agent of the organization, whatever the filters show, including the agents their platforms no longer list; it returns how many agents it deleted. Platforms and scan history are kept, and nothing changes on the platforms: a later scan lists an agent again if a platform still has it.

**Only agents that exist.** When every listing of a connection succeeds completely, an agent of that connection the scan did not return is marked as no longer listed (`present: false`): it was deleted at the platform, or is no longer in the connection's scope (a removed project key, region or service). The tab shows only agents that are still listed. A scan with a failed or incomplete listing (an error, a refused permission, a page limit) marks nothing, so a platform error never hides an agent.

**Evidence.** Each agent shows the listing that returned it, the exact condition in the platform's field names, and what was observed (such as a runtime's status). A Foundry *external* agent is also flagged as a *possible duplicate* of an agent from another platform with the same name (Foundry stores no endpoint for external agents, so they are never merged).

**Details.** An agent shows what its listing reports, through an allow-list: model, framework, kind, version, status, tools and MCP servers (type and name; server label and URL origin), skills, image, source repository, region or location, project or workspace, execution role, protocol, labels and the platform's short description. Owner details are shown only when the agent listing reports who created the agent.

## Data retention

| Data | Kept |
|---|---|
| Platform secrets | Never in plain text: stored AES-GCM encrypted with `ENCRYPTION_KEY`, never returned, logged or audited; the API shows only a redacted hint |
| Instructions, system prompts, tool schemas and definitions, environment variable values, MCP headers, descriptions marked sensitive | Never decoded, stored or logged |
| Agent details | The allow-listed attributes above; MCP and agent URLs reduced to scheme and host |
| Evidence | Evidence code, source call and observed flags and labels |

Error messages are scrubbed of anything that looks like a credential.

## API

All under `/api/v1/orgs/{orgName}/shadow-agents` (see `docs/api_v1_openapi.yaml`, tag *Shadow Agent Discovery*): `providers`, `summary`, `connections` (+ `/{id}`, `/{id}/test`), `scans` (+ `/{id}`; `DELETE` clears the finished scans), `findings` (+ `/{id}`).

## Limits

- An agent is found only where the connection can list it: an OpenAI project or Anthropic workspace without a key, a region or location not configured, or an unticked service is not listed.
- Agent Manager cannot tell which OpenAI projects or Anthropic workspaces have no key: add a key for each one to scan.
- Some platform fields (such as the AWS Agent Registry provenance and the Google `RuntimeReference`) are not documented exhaustively; connectors parse them leniently and record a call as `unsupported_response` when a response no longer has the documented shape.

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

/**
 * The console's map of the product documentation.
 *
 * Every screen points at the page that explains the thing it manages, so the
 * "what is this?" question has an answer one click away rather than a search
 * away. Paths mirror the documentation site's own file layout under
 * `docsUrl` — `concepts/project` is served at `<docsUrl>/concepts/project/`.
 *
 * Adding a screen means adding its entry here rather than hand-building a URL
 * at the call site: the ids are typo-checked by the compiler, and a doc page
 * that moves is repointed once.
 */

/** A single documentation page the console links to. */
export interface DocEntry {
  /** Path under the docs base URL. No leading or trailing slash. */
  path: string;
  /** The page's own title, as it reads in the documentation. */
  title: string;
  /**
   * A 1–3 word label for the in-app link. Kept short because it sits in page
   * headers beside the primary action, where a full title would crowd it out.
   */
  shortTitle: string;
  /** One line on what the page covers. Shown in tooltips and callouts. */
  summary: string;
}

export const DOCS = {
  // ---------------------------------------------------------------- Get started
  whatIsAmp: {
    path: 'get-started/what-is-amp',
    title: 'WSO2 Agent Manager',
    shortTitle: 'Overview',
    summary:
      'An open control plane for enterprises to deploy, manage, and govern AI agents at scale.',
  },
  quickStart: {
    path: 'get-started/quick-start',
    title: 'Quick Start Guide',
    shortTitle: 'Quick start',
    summary: 'Get Agent Manager running with a single command using a dev container.',
  },

  // ------------------------------------------------------------------ Concepts
  organization: {
    path: 'concepts/organization',
    title: 'Organization',
    shortTitle: 'Organizations',
    summary:
      'The top-level tenant boundary — every project, environment, gateway, and resource belongs to exactly one organization.',
  },
  project: {
    path: 'concepts/project',
    title: 'Project',
    shortTitle: 'Projects',
    summary:
      'The container for your agents, and where a deployment pipeline is attached to govern how they promote across environments.',
  },
  agentLifecycle: {
    path: 'concepts/agent-lifecycle',
    title: 'Agent and Agent Life Cycle',
    shortTitle: 'Agents',
    summary:
      'How an agent is registered, built, deployed, governed, and observed through the platform.',
  },
  internalAndExternalAgent: {
    path: 'concepts/internal-and-external-agent',
    title: 'Internal and External Agent',
    shortTitle: 'Agent types',
    summary:
      'Platform-Hosted vs Externally-Hosted agents — chosen at creation and not changeable afterward.',
  },
  agentKindAndCatalog: {
    path: 'concepts/agent-kind-and-catalog',
    title: 'Agent Kind & Agent Catalog',
    shortTitle: 'Agent kinds',
    summary:
      'Reusable agent templates and the catalog teams pick from when creating a new agent.',
  },
  agentSandboxing: {
    path: 'concepts/agent-sandboxing',
    title: 'Agent Sandboxing',
    shortTitle: 'Sandboxing',
    summary:
      "How Agent Manager isolates a running agent's code from the host, through per-environment runtime tiers.",
  },
  agentId: {
    path: 'concepts/agentid',
    title: 'AgentID',
    shortTitle: 'AgentID',
    summary:
      'The identity every agent carries when it calls an MCP tool through a gateway.',
  },
  deploymentPipeline: {
    path: 'concepts/deployment-pipeline',
    title: 'Deployment Pipeline',
    shortTitle: 'Pipelines',
    summary:
      'The ordered set of environments agents in a project are allowed to move through.',
  },
  environment: {
    path: 'concepts/environment',
    title: 'Environment',
    shortTitle: 'Environments',
    summary:
      'An isolated space where agents run and resources deploy — development, staging, production.',
  },
  evaluation: {
    path: 'concepts/evaluation',
    title: 'Evaluation',
    shortTitle: 'Evaluation',
    summary:
      'Running evaluators against execution traces to produce agent quality scores you can track over time.',
  },
  gateway: {
    path: 'concepts/gateway',
    title: 'Gateway',
    shortTitle: 'Gateways',
    summary:
      'The controlled proxy every LLM service provider and MCP proxy is exposed to agents through.',
  },
  llmServiceProvider: {
    path: 'concepts/llm-service-provider',
    title: 'LLM Service Provider',
    shortTitle: 'LLM providers',
    summary:
      'Organization-level connections to upstream LLM APIs, exposed through an AI gateway.',
  },
  mcpProxy: {
    path: 'concepts/mcp-proxy',
    title: 'MCP Proxy',
    shortTitle: 'MCP servers',
    summary:
      'An organization-level resource fronting upstream MCP servers, discovering their tools and re-exposing them per environment.',
  },
  observability: {
    path: 'concepts/observability',
    title: 'Observability',
    shortTitle: 'Observability',
    summary:
      'Traces, metrics, and logs from platform-hosted and external agents, in one centralized store.',
  },

  // -------------------------------------------------------------------- Guides
  environmentManagement: {
    path: 'guides/environment-management',
    title: 'Manage Environments and Deployment Pipelines',
    shortTitle: 'Manage environments',
    summary:
      'Control where agents run and how they are promoted from one stage to the next.',
  },
  registerLlmServiceProvider: {
    path: 'guides/register-llm-service-provider',
    title: 'Register an LLM Service Provider',
    shortTitle: 'Register a provider',
    summary:
      'Connect an upstream LLM API so agents across the organization can use it.',
  },
  registerMcpProxy: {
    path: 'guides/register-mcp-proxy',
    title: 'Register an MCP Proxy',
    shortTitle: 'Register a server',
    summary:
      'Front an upstream MCP server, discover its tools, and expose them per environment.',
  },
  configureAgentLlm: {
    path: 'guides/configure-agent-llm-configuration',
    title: 'Configure LLM Providers for an Agent',
    shortTitle: 'LLM setup',
    summary:
      'Attach organization-level LLM service providers to a platform-hosted or external agent.',
  },
  configureAgentMcpProxies: {
    path: 'guides/configure-agent-mcp-proxies',
    title: 'Configure MCP Proxies for an Agent',
    shortTitle: 'Tool setup',
    summary:
      "Bind an org-level MCP proxy to an agent and expose the proxy's per-environment endpoint to it.",
  },
  authorizeAgentMcpTools: {
    path: 'guides/authorize-agent-access-to-mcp-tools',
    title: 'Authorize Agent Access to MCP Tools',
    shortTitle: 'Tool authorization',
    summary:
      "Define permissions over an MCP proxy's tools, bundle them into roles, and decide which agents may invoke what.",
  },
  secureAgentsWithApiKeys: {
    path: 'guides/secure-agent-endpoints-with-api-keys',
    title: 'Secure Agent Endpoints with API Keys',
    shortTitle: 'API key security',
    summary:
      'Protect platform-hosted agent endpoints at the gateway with a key sent in the X-API-Key header.',
  },
  secureAgentsWithJwt: {
    path: 'guides/secure-agents-with-jwt-authentication',
    title: 'Secure Agents with JWT Authentication',
    shortTitle: 'JWT security',
    summary:
      'Protect platform-hosted agent endpoints at the gateway with JWT-based authentication.',
  },
  configureCors: {
    path: 'guides/configure-cors-for-agent-endpoints',
    title: 'Configure CORS for Agent Endpoints',
    shortTitle: 'CORS',
    summary:
      'Control which browser origins, methods, and headers may reach a platform-hosted agent endpoint.',
  },
  configureIdentityProviders: {
    path: 'guides/configure-identity-providers-at-the-gateway',
    title: 'Configure Identity Providers at the Gateway',
    shortTitle: 'Identity providers',
    summary: 'Set the token issuers your gateway trusts when an agent endpoint is secured.',
  },
  useAgentIdInPlatformHosted: {
    path: 'guides/use-agentid-in-platform-hosted-agents',
    title: 'Use AgentID in a Platform-Hosted Agent',
    shortTitle: 'Use AgentID',
    summary:
      'Every platform-hosted agent receives its own AgentID — how to use it from inside the workload.',
  },
  retrieveAgentIdForExternal: {
    path: 'guides/retrieve-agentid-for-externally-hosted-agents',
    title: 'Retrieve AgentID Credentials for an Externally-Hosted Agent',
    shortTitle: 'AgentID credentials',
    summary:
      'Fetch the credentials an externally-hosted agent needs, since the platform has no pod to inject them into.',
  },
  evaluationMonitors: {
    path: 'guides/evaluation-monitors',
    title: 'Evaluation Monitors',
    shortTitle: 'Monitors',
    summary: 'Create an evaluation monitor, view its results, and manage it over time.',
  },
  customEvaluators: {
    path: 'guides/custom-evaluators',
    title: 'Custom Evaluators',
    shortTitle: 'Evaluators',
    summary:
      'Define domain-specific quality checks with Python code or LLM judge prompt templates.',
  },
  ampInstrumentation: {
    path: 'guides/amp-instrumentation',
    title: 'WSO2 Agent Manager Instrumentation Package',
    shortTitle: 'Instrumentation',
    summary:
      'Zero-code OpenTelemetry instrumentation for externally-hosted Python agents, with traces in the console.',
  },
  configureTraceSampling: {
    path: 'guides/configure-trace-sampling',
    title: 'Configure Trace Sampling',
    shortTitle: 'Trace sampling',
    summary:
      'Keep trace export practical at high request rates by sampling rather than exporting everything.',
  },
  cliInstallation: {
    path: 'guides/cli-installation',
    title: 'CLI Installation',
    shortTitle: 'CLI',
    summary:
      'Install amctl to manage organizations, projects, agents, and observability from your terminal.',
  },

  // ----------------------------------------------------------------- Tutorials
  createFirstAgent: {
    path: 'tutorials/create-your-first-agent',
    title: 'Create Your First Agent',
    shortTitle: 'Tutorial',
    summary: 'Walk through creating your first agent from the console, end to end.',
  },
  observeFirstAgent: {
    path: 'tutorials/observe-first-agent',
    title: 'Observe Your First Agent',
    shortTitle: 'Tutorial',
    summary:
      'Register an external agent, connect zero-code instrumentation, and view its traces in the console.',
  },

  // ----------------------------------------------------------------- Reference
  authorization: {
    path: 'reference/authorization',
    title: 'Authorization',
    shortTitle: 'Permissions',
    summary: 'The full permission model — scopes, roles, and what each one grants.',
  },
} as const satisfies Record<string, DocEntry>;

/** Identifier of a page in {@link DOCS}. */
export type DocId = keyof typeof DOCS;

/**
 * A documentation target: either a bare page id, or a page plus the heading
 * anchor and link label to use for it.
 */
export type DocsTarget =
  | DocId
  | {
      id: DocId;
      /** Heading anchor on the page, without the leading `#`. */
      anchor?: string;
      /** Overrides the entry's `shortTitle` as the link label. */
      label?: string;
    };

/** Splits a {@link DocsTarget} into the entry it names and its overrides. */
export function resolveDocsTarget(target: DocsTarget): {
  entry: DocEntry;
  anchor?: string;
  label: string;
} {
  const { id, anchor, label } =
    typeof target === 'string' ? { id: target, anchor: undefined, label: undefined } : target;
  const entry = DOCS[id];
  return { entry, anchor, label: label ?? entry.shortTitle };
}

/**
 * The configured documentation base URL, read straight off the runtime config.
 *
 * `globalConfig` isn't imported for this: it lives in a package this one
 * depends on for types only, and pulling in a runtime value from there would
 * bundle that whole package — and its own dependencies — into this one.
 */
function docsBaseUrl(): string | undefined {
  if (typeof window === 'undefined') return undefined;
  const config = (window as { __RUNTIME_CONFIG__?: { docsUrl?: string } }).__RUNTIME_CONFIG__;
  return config?.docsUrl?.trim() || undefined;
}

/**
 * Absolute URL of a documentation page, or `undefined` when this deployment
 * has no `docsUrl` configured.
 *
 * Callers render nothing on `undefined` rather than falling back to a guessed
 * host: an install that deliberately ships without public docs should show no
 * link at all, not a broken one.
 */
export function docsHref(target: DocsTarget): string | undefined {
  const baseUrl = docsBaseUrl();
  if (!baseUrl) return undefined;

  const { entry, anchor } = resolveDocsTarget(target);
  // The documentation site is served with trailing slashes.
  const base = `${baseUrl.replace(/\/+$/, '')}/${entry.path}/`;
  return anchor ? `${base}#${anchor}` : base;
}

/**
 * URL of the documentation's landing page, or `undefined` when no `docsUrl` is
 * configured. Used for the "browse everything" escape hatch, for the times the
 * page someone needs isn't one this console thought to link.
 */
export function docsHomeHref(): string | undefined {
  const baseUrl = docsBaseUrl();
  if (!baseUrl) return undefined;
  return `${baseUrl.replace(/\/+$/, '')}/${DOCS.whatIsAmp.path}/`;
}

/** Whether this deployment has documentation links configured at all. */
export function isDocsConfigured(): boolean {
  return docsBaseUrl() !== undefined;
}

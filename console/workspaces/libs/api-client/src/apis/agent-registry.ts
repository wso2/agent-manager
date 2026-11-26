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

import type {
  AgentRegistryA2ASkill,
  AgentRegistryEntry,
  AgentRegistryEnvironment,
  AgentRegistryFindingSignal,
  AgentRegistryListResponse,
  AgentRegistryMcpServerRef,
  AgentRegistryMonitorRef,
  AgentRegistryResourceRef,
  GetAgentRegistryEntryPathParams,
  GetUnregisteredAgentPathParams,
  ListAgentRegistryPathParams,
} from "@agent-management-platform/types";

// SAMPLE DATA. The registry API (GET /orgs/{org}/agents with registry fields,
// issue #78) does not exist yet. Unregistered agents will come from Shadow Agent
// Discovery (issue #71). When it does, replace the bodies of listAgentRegistry
// and getAgentRegistryEntry with httpGET calls and delete the sample data below.
// "createdBy" stands in for the owner: owners do not exist yet.

const LLM = {
  openai: { id: "openai-gpt", displayName: "OpenAI GPT" },
  anthropic: { id: "anthropic-claude", displayName: "Anthropic Claude" },
  azure: { id: "azure-openai", displayName: "Azure OpenAI" },
} satisfies Record<string, AgentRegistryResourceRef>;

const MCP = {
  payments: {
    id: "payments-mcp",
    displayName: "Payments",
    tools: ["get_invoice", "create_refund"],
  },
  github: {
    id: "github-mcp",
    displayName: "GitHub",
    tools: ["search_issues", "get_pull_request"],
  },
  jira: {
    id: "jira-mcp",
    displayName: "Jira",
    tools: ["search_issues", "create_issue"],
  },
  hotels: {
    id: "hotel-search-mcp",
    displayName: "Hotel search",
    tools: ["search_hotels", "book_room"],
  },
  hr: {
    id: "hr-records-mcp",
    displayName: "HR records",
    tools: ["get_leave_balance"],
  },
} satisfies Record<string, AgentRegistryMcpServerRef>;

const BOOKING_SKILLS: AgentRegistryA2ASkill[] = [
  {
    id: "book-hotel",
    name: "Book a hotel",
    description:
      'Finds hotels near the trip\'s destination within the travel policy\'s nightly limit and books the chosen room. Select it with message metadata {"skill": "book-hotel"} or a "Book hotel:" text prefix; it is also the default.',
    tags: ["travel", "hotel"],
  },
  {
    id: "book-flight",
    name: "Book a flight",
    description:
      'Searches economy fares for the given dates and route, holds the cheapest one that fits the policy and books it once confirmed. Select it with message metadata {"skill": "book-flight"} or a "Book flight:" text prefix.',
    tags: ["travel", "flight"],
  },
];

/** Governance config of one environment; anything left out is empty. */
interface Governance {
  /** Defaults to "completed" once deployed, else null. */
  agentId?: AgentRegistryEnvironment["agentIdStatus"];
  guardrails?: string[];
  monitors?: AgentRegistryMonitorRef[];
  /** Defaults to an API key and no OAuth. */
  inboundSecurity?: AgentRegistryEnvironment["inboundSecurity"];
  /** The A2A card's skills fetched in this environment. */
  a2aSkills?: AgentRegistryA2ASkill[];
}

function minutesAgo(minutes: number): string {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}

function dev(
  status: AgentRegistryEnvironment["deploymentStatus"],
  llmProviders: AgentRegistryResourceRef[],
  mcpServers: AgentRegistryMcpServerRef[],
  deployedMinutesAgo?: number,
  governance: Governance = {},
): AgentRegistryEnvironment {
  return {
    name: "development",
    displayName: "Development",
    isProduction: false,
    deploymentStatus: status,
    // External agents pass no time: Agent Manager never deploys them.
    lastDeployedAt:
      status === "not-deployed" || deployedMinutesAgo === undefined
        ? null
        : minutesAgo(deployedMinutesAgo),
    agentIdStatus:
      governance.agentId !== undefined
        ? governance.agentId
        : status === "not-deployed"
          ? null
          : "completed",
    llmProviders,
    guardrails: governance.guardrails ?? [],
    mcpServers,
    monitors: governance.monitors ?? [],
    inboundSecurity: governance.inboundSecurity ?? {
      apiKey: true,
      oauth: false,
    },
    a2aCard: governance.a2aSkills
      ? { skills: governance.a2aSkills }
      : undefined,
  };
}

function prod(
  status: AgentRegistryEnvironment["deploymentStatus"],
  llmProviders: AgentRegistryResourceRef[],
  mcpServers: AgentRegistryMcpServerRef[],
  deployedMinutesAgo?: number,
  governance: Governance = {},
): AgentRegistryEnvironment {
  return {
    name: "production",
    displayName: "Production",
    isProduction: true,
    deploymentStatus: status,
    // External agents pass no time: Agent Manager never deploys them.
    lastDeployedAt:
      status === "not-deployed" || deployedMinutesAgo === undefined
        ? null
        : minutesAgo(deployedMinutesAgo),
    agentIdStatus:
      governance.agentId !== undefined
        ? governance.agentId
        : status === "not-deployed"
          ? null
          : "completed",
    llmProviders,
    guardrails: governance.guardrails ?? [],
    mcpServers,
    monitors: governance.monitors ?? [],
    inboundSecurity: governance.inboundSecurity ?? {
      apiKey: true,
      oauth: false,
    },
    a2aCard: governance.a2aSkills
      ? { skills: governance.a2aSkills }
      : undefined,
  };
}

// Connection and finding IDs are UUIDs in the API (ShadowFinding.ID, ConnectionID).
const OPENAI_PROD = "5b1c7e2a-3f4d-4a8e-9c61-2d7f0b9e4a13";
const AWS_PROD = "8e2d4f61-7a3b-4c9d-b5e0-1f6a2c8d7e94";
const GOOGLE_PROD = "c4a91d37-2e5f-4b80-8d1c-6f3e9a7b2c05";

interface ShadowSample {
  findingId: string;
  displayName: string;
  provider: string;
  providerDisplayName: string;
  connection: { id: string; name: string };
  kind: "platform_agent" | "runtime";
  externalId: string;
  scope: string;
  state: string;
  providerCreatedAt?: string;
  attributes: Record<string, unknown>;
  signal: Omit<AgentRegistryFindingSignal, "detectedAt">;
  firstSeenMinutesAgo: number;
  lastSeenMinutesAgo: number;
  present?: boolean;
  onboarded?: boolean;
}

/** A registry entry for one Shadow Agent Discovery finding. */
function shadowAgent(f: ShadowSample): AgentRegistryEntry {
  const present = f.present ?? true;
  // One connection per sample: nothing on the branch shows these agents found twice.
  const foundBy = [
    {
      findingId: f.findingId,
      connectionId: f.connection.id,
      connectionName: f.connection.name,
      present,
    },
  ];
  return {
    name: f.findingId,
    displayName: f.displayName,
    registration: "unregistered",
    discovery: {
      findingId: f.findingId,
      provider: f.provider,
      providerDisplayName: f.providerDisplayName,
      scope: f.scope,
      ownerHint: {},
      firstSeenAt: minutesAgo(f.firstSeenMinutesAgo),
      lastSeenAt: minutesAgo(f.lastSeenMinutesAgo),
      present,
      onboarded: f.onboarded ?? false,
      connectionName: f.connection.name,
      kind: f.kind,
      externalId: f.externalId,
      state: f.state,
      providerCreatedAt: f.providerCreatedAt,
      foundBy,
      attributes: f.attributes,
      signals: [{ ...f.signal, detectedAt: minutesAgo(f.lastSeenMinutesAgo) }],
    },
    projectName: "",
    projectDisplayName: "",
    hosting: "external",
    interface: null,
    createdBy: null,
    labels: {},
    environments: [],
    createdAt: minutesAgo(f.firstSeenMinutesAgo),
  };
}

function sampleAgents(): AgentRegistryEntry[] {
  return [
    {
      name: "booking-agent",
      displayName: "Booking agent",
      description: "Books hotels and flights for employees on business travel.",
      registration: "registered",
      projectName: "travel",
      projectDisplayName: "Travel",
      hosting: "platform",
      interface: "a2a-agent",
      createdBy: { id: "u-sanduni", displayName: "Sanduni Jayasuriya" },
      labels: { team: "travel", "data-class": "confidential" },
      environments: [
        dev("active", [LLM.openai], [MCP.hotels, MCP.payments], 1200, {
          guardrails: ["pii-masking"],
          a2aSkills: [
            ...BOOKING_SKILLS,
            {
              id: "cancel-booking",
              name: "Cancel a booking",
              description:
                'Cancels a hotel or flight booking by its reference and reports any cancellation fee before confirming. Select it with message metadata {"skill": "cancel-booking"} or a "Cancel:" text prefix.',
              tags: ["travel", "cancel"],
            },
          ],
        }),
        prod("active", [LLM.openai], [MCP.hotels, MCP.payments], 2880, {
          guardrails: ["pii-masking", "word-count-guardrail"],
          monitors: [
            {
              name: "booking-quality",
              displayName: "Booking quality",
              status: "Active",
            },
          ],
          a2aSkills: BOOKING_SKILLS,
        }),
      ],
      createdAt: minutesAgo(60 * 24 * 120),
    },
    {
      name: "expense-checker",
      displayName: "Expense checker",
      description: "Checks expense claims against the travel policy.",
      registration: "registered",
      projectName: "finance",
      projectDisplayName: "Finance",
      hosting: "platform",
      interface: "custom-api",
      kindName: "policy-checker",
      kindVersion: "1.2.0",
      createdBy: { id: "u-nimal", displayName: "Nimal Perera" },
      labels: { team: "finance" },
      environments: [
        dev("active", [LLM.azure], [MCP.payments], 300),
        prod("active", [LLM.azure], [MCP.payments], 8640, {
          guardrails: ["content-length-guardrail"],
        }),
      ],
      createdAt: minutesAgo(60 * 24 * 90),
    },
    {
      name: "invoice-reader",
      displayName: "Invoice reader",
      description: "Extracts line items from supplier invoices.",
      registration: "registered",
      projectName: "finance",
      projectDisplayName: "Finance",
      hosting: "external",
      interface: "custom-api",
      createdBy: null,
      labels: {},
      environments: [
        dev("active", [LLM.anthropic], []),
        prod("active", [LLM.anthropic], [], undefined, {
          inboundSecurity: { apiKey: false, oauth: false },
        }),
      ],
      createdAt: minutesAgo(60 * 24 * 200),
    },
    {
      name: "support-chat",
      displayName: "Support chat",
      description: "Answers customer questions from the help centre.",
      registration: "registered",
      projectName: "support",
      projectDisplayName: "Customer support",
      hosting: "platform",
      interface: "chat-api",
      createdBy: { id: "u-ravindu", displayName: "Ravindu Wickramasinghe" },
      labels: { team: "support", channel: "web" },
      environments: [
        dev("active", [LLM.anthropic], [MCP.jira], 120),
        prod("active", [LLM.anthropic], [MCP.jira], 1440, {
          guardrails: ["word-count-guardrail"],
          monitors: [
            {
              name: "support-answer-quality",
              displayName: "Answer quality",
              status: "Active",
            },
          ],
          inboundSecurity: { apiKey: true, oauth: true },
        }),
      ],
      createdAt: minutesAgo(60 * 24 * 300),
    },
    {
      name: "ticket-triage",
      displayName: "Ticket triage",
      description: "Labels and routes new support tickets.",
      registration: "registered",
      projectName: "support",
      projectDisplayName: "Customer support",
      hosting: "platform",
      interface: "custom-api",
      createdBy: { id: "u-amaya", displayName: "Amaya Silva" },
      labels: { team: "support" },
      environments: [
        dev("suspended", [LLM.openai], [MCP.jira], 43200),
        prod("active", [LLM.openai], [MCP.jira], 57600, {
          monitors: [
            {
              name: "triage-accuracy",
              displayName: "Triage accuracy",
              status: "Suspended",
            },
          ],
        }),
      ],
      createdAt: minutesAgo(60 * 24 * 400),
    },
    {
      name: "support-chat-v1",
      displayName: "Support chat (v1)",
      description: "First support assistant, replaced by Support chat.",
      registration: "registered",
      projectName: "support",
      projectDisplayName: "Customer support",
      hosting: "platform",
      interface: "chat-api",
      createdBy: { id: "u-ravindu", displayName: "Ravindu Wickramasinghe" },
      labels: { team: "support" },
      environments: [
        dev("not-deployed", [], []),
        prod("suspended", [LLM.openai], [], 172800),
      ],
      createdAt: minutesAgo(60 * 24 * 600),
    },
    {
      name: "code-reviewer",
      displayName: "Code reviewer",
      description: "Reviews pull requests and suggests fixes.",
      registration: "registered",
      projectName: "engineering",
      projectDisplayName: "Engineering",
      hosting: "platform",
      interface: "a2a-agent",
      kindName: "reviewer",
      kindVersion: "0.4.1",
      createdBy: { id: "u-kasun", displayName: "Kasun Fernando" },
      labels: { team: "devex" },
      environments: [
        dev("in-progress", [LLM.anthropic], [MCP.github], 30, {
          agentId: "in_progress",
          a2aSkills: [
            {
              id: "review-pr",
              name: "Review a pull request",
              description:
                'Reads a pull request\'s diff and comments, flags likely bugs and missing tests, and suggests fixes as review comments. Select it with message metadata {"skill": "review-pr"} or a "Review:" text prefix; it is also the default.',
              tags: ["code", "review"],
            },
          ],
        }),
        prod("not-deployed", [], []),
      ],
      createdAt: minutesAgo(60 * 5),
    },
    {
      name: "release-notes",
      displayName: "Release notes writer",
      description: "Drafts release notes from merged pull requests.",
      registration: "registered",
      projectName: "engineering",
      projectDisplayName: "Engineering",
      hosting: "external",
      interface: "a2a-agent",
      createdBy: null,
      labels: { team: "devex" },
      environments: [
        dev("active", [LLM.openai], [MCP.github, MCP.jira]),
        prod("active", [LLM.openai], [MCP.github, MCP.jira]),
      ],
      createdAt: minutesAgo(60 * 24 * 30),
    },
    {
      name: "leave-assistant",
      displayName: "Leave assistant",
      description: "Answers leave questions and files leave requests.",
      registration: "registered",
      projectName: "hr",
      projectDisplayName: "HR",
      hosting: "platform",
      interface: "chat-api",
      createdBy: { id: "u-tharushi", displayName: "Tharushi Fernando" },
      labels: { team: "hr", "data-class": "personal" },
      environments: [
        dev("active", [LLM.azure], [MCP.hr], 180),
        prod("failed", [LLM.azure], [MCP.hr], 540, {
          agentId: "failed",
          guardrails: ["pii-masking"],
        }),
      ],
      createdAt: minutesAgo(60 * 24 * 60),
    },
    {
      name: "payroll-qa",
      displayName: "Payroll Q&A",
      description: "Answers payslip questions.",
      registration: "registered",
      projectName: "hr",
      projectDisplayName: "HR",
      hosting: "platform",
      interface: "chat-api",
      createdBy: null,
      labels: { team: "hr", "data-class": "personal" },
      environments: [
        dev("failed", [LLM.azure], [MCP.hr, MCP.payments], 1440),
        prod("not-deployed", [], []),
      ],
      createdAt: minutesAgo(60 * 24 * 2),
    },
    // Unregistered: shadow agents found by Shadow Agent Discovery (issue #71). Every
    // value comes from the shadow-agent-registry branch: its connectors' test data
    // (connectors/*/testdata), signal catalog (signals.go), provider names (catalog.go)
    // and the connection names its console tests use. No connector sets an owner hint
    // yet, so ownerHint is empty throughout.
    shadowAgent({
      findingId: "0f6c2b8e-4d1a-4e7b-a3c9-5e2f8d1b6a47",
      displayName: "Support agent",
      provider: "openai",
      providerDisplayName: "OpenAI",
      connection: { id: OPENAI_PROD, name: "OpenAI prod" },
      kind: "platform_agent",
      externalId: "agent_alpha",
      scope: "proj_alpha",
      state: "active",
      providerCreatedAt: "2026-10-31T00:00:00Z",
      attributes: {
        model: "gpt-x",
        toolCount: 4,
        tools: [
          { type: "function", name: "lookup_order" },
          { type: "mcp", name: "github" },
          { type: "mcp", name: "docs" },
          { type: "web_search" },
        ],
        mcpServers: [
          { label: "github", origin: "https://mcp.example.com" },
          { label: "docs" },
        ],
      },
      signal: {
        code: "openai.agent_object",
        name: "Agent object",
        source: "GET /v1/agents",
        condition:
          "GET /v1/agents (OpenAI-Beta: agents=v1, project key): an item in data",
        observed: { toolCount: 4, mcpServerCount: 2 },
      },
      firstSeenMinutesAgo: 60 * 24 * 14,
      lastSeenMinutesAgo: 60 * 6,
    }),
    shadowAgent({
      findingId: "3a9e5d17-8b2c-4f60-9e4a-7c1d3b5f8e22",
      displayName: "billing-helper",
      provider: "bedrock",
      providerDisplayName: "Amazon Bedrock",
      connection: { id: AWS_PROD, name: "AWS prod" },
      kind: "platform_agent",
      externalId: "bedrock:us-east-1:agent/AB12CD34EF",
      scope: "us-east-1",
      state: "PREPARED",
      providerCreatedAt: "2026-10-01T12:00:00Z",
      attributes: {
        region: "us-east-1",
        status: "PREPARED",
        agentVersion: "2",
        resourceId: "arn:aws:bedrock:us-east-1:123456789012:agent/AB12CD34EF",
        model: "anthropic.claude-3-5-sonnet-20240620-v1:0",
        orchestrationType: "DEFAULT",
        roleArn:
          "arn:aws:iam::123456789012:role/service-role/AmazonBedrockExecutionRoleForAgents_billing",
        tools: [
          { type: "action_group", name: "lookup_invoice" },
          { type: "action_group", name: "UserInputAction" },
        ],
        toolCount: 2,
      },
      signal: {
        code: "bedrock.classic_agent",
        name: "Classic Bedrock agent",
        source: "Bedrock Agents ListAgents",
        condition: "bedrock-agent ListAgents: an item in agentSummaries",
        observed: { agentStatus: "PREPARED" },
      },
      firstSeenMinutesAgo: 60 * 24 * 3,
      lastSeenMinutesAgo: 60 * 6,
    }),
    shadowAgent({
      findingId: "9d4b7e2c-1f8a-4c35-b6d0-2e7a9c4f1b58",
      displayName: "support-agent",
      provider: "google",
      providerDisplayName: "Google Agent Platform",
      // The branch's tests name no Google connection; this one name is a placeholder.
      connection: { id: GOOGLE_PROD, name: "Google prod" },
      kind: "runtime",
      externalId:
        "projects/123456789012/locations/us-central1/reasoningEngines/1234567890123456789",
      scope: "projects/my-agents-project/locations/us-central1",
      // The Google connector does not set a state for engines.
      state: "",
      providerCreatedAt: "2026-10-01T10:00:00.123456Z",
      attributes: {
        project: "my-agents-project",
        location: "us-central1",
        labels: { team: "cx" },
        framework: "google-adk",
        serviceAccount:
          "agents.global.org-111111111111.system.id.goog/resources/aiplatform/projects/123456789012/locations/us-central1/reasoningEngines/1234567890123456789",
      },
      signal: {
        code: "google.agent_runtime_engine",
        name: "Agent Runtime engine",
        source:
          "GET /v1/projects/{project}/locations/{location}/reasoningEngines",
        condition:
          "reasoningEngines.list: an item in reasoningEngines; spec.agentFramework names the framework",
        observed: { frameworkSet: true, gitSource: false },
      },
      firstSeenMinutesAgo: 60 * 24 * 40,
      lastSeenMinutesAgo: 60 * 30,
    }),
    // Not listed: onboarded as an external agent, so it shows under Registered.
    shadowAgent({
      findingId: "6e1f3a9c-5b7d-4280-8c2e-4a9d1f7b3e60",
      displayName: "Beta one",
      provider: "openai",
      providerDisplayName: "OpenAI",
      connection: { id: OPENAI_PROD, name: "OpenAI prod" },
      kind: "platform_agent",
      externalId: "agent_b1",
      scope: "proj_beta",
      state: "active",
      providerCreatedAt: "2026-10-31T00:00:00Z",
      attributes: { model: "gpt-y" },
      signal: {
        code: "openai.agent_object",
        name: "Agent object",
        source: "GET /v1/agents",
        condition:
          "GET /v1/agents (OpenAI-Beta: agents=v1, project key): an item in data",
        observed: { toolCount: 0, mcpServerCount: 0 },
      },
      firstSeenMinutesAgo: 60 * 24 * 14,
      lastSeenMinutesAgo: 60 * 6,
      onboarded: true,
    }),
    // Not listed: its platform no longer lists it.
    shadowAgent({
      findingId: "b2c8e4f1-9a3d-4d76-a1e5-8f2b6c9d4a31",
      displayName: "hr-bot",
      provider: "bedrock",
      providerDisplayName: "Amazon Bedrock",
      connection: { id: AWS_PROD, name: "AWS prod" },
      kind: "platform_agent",
      externalId: "bedrock:us-east-1:agent/XY98ZW76VU",
      scope: "us-east-1",
      state: "NOT_PREPARED",
      providerCreatedAt: "2026-10-02T12:00:00Z",
      attributes: { region: "us-east-1", status: "NOT_PREPARED" },
      signal: {
        code: "bedrock.classic_agent",
        name: "Classic Bedrock agent",
        source: "Bedrock Agents ListAgents",
        condition: "bedrock-agent ListAgents: an item in agentSummaries",
        observed: { agentStatus: "NOT_PREPARED" },
      },
      firstSeenMinutesAgo: 60 * 24 * 30,
      lastSeenMinutesAgo: 60 * 24 * 10,
      present: false,
    }),
  ];
}

/** Lists every agent in the organization with its registry record. Returns sample data for now. */
export async function listAgentRegistry(
  params: ListAgentRegistryPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentRegistryListResponse> {
  // Kept so the signature matches the real call that will replace this body.
  void params;
  void getToken;
  return { agents: sampleAgents() };
}

/**
 * One registered agent's registry record, or null when there is no such agent.
 * Returns sample data for now.
 */
export async function getAgentRegistryEntry(
  params: GetAgentRegistryEntryPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentRegistryEntry | null> {
  void getToken;
  const agent = sampleAgents().find(
    (a) =>
      a.registration === "registered" &&
      a.projectName === params.projName &&
      a.name === params.agentName,
  );
  return agent ?? null;
}

/**
 * One unregistered agent by its finding ID, or null when there is none. Returns sample
 * data for now; it will read the Shadow Agent Discovery finding.
 */
export async function getUnregisteredAgent(
  params: GetUnregisteredAgentPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentRegistryEntry | null> {
  void getToken;
  const agent = sampleAgents().find(
    (a) =>
      a.registration === "unregistered" &&
      a.discovery?.findingId === params.findingId,
  );
  return agent ?? null;
}

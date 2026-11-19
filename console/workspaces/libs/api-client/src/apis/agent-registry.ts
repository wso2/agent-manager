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
  AgentRegistryEntry,
  AgentRegistryEnvironment,
  AgentRegistryListResponse,
  AgentRegistryResourceRef,
  GetAgentRegistryEntryPathParams,
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
  payments: { id: "payments-mcp", displayName: "Payments" },
  github: { id: "github-mcp", displayName: "GitHub" },
  jira: { id: "jira-mcp", displayName: "Jira" },
  hotels: { id: "hotel-search-mcp", displayName: "Hotel search" },
  hr: { id: "hr-records-mcp", displayName: "HR records" },
} satisfies Record<string, AgentRegistryResourceRef>;

function minutesAgo(minutes: number): string {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}

function dev(
  status: AgentRegistryEnvironment["deploymentStatus"],
  llmProviders: AgentRegistryResourceRef[],
  mcpServers: AgentRegistryResourceRef[],
  deployedMinutesAgo?: number,
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
    llmProviders,
    mcpServers,
  };
}

function prod(
  status: AgentRegistryEnvironment["deploymentStatus"],
  llmProviders: AgentRegistryResourceRef[],
  mcpServers: AgentRegistryResourceRef[],
  deployedMinutesAgo?: number,
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
    llmProviders,
    mcpServers,
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
        dev("active", [LLM.openai], [MCP.hotels, MCP.payments], 1200),
        prod("active", [LLM.openai], [MCP.hotels, MCP.payments], 2880),
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
        prod("active", [LLM.azure], [MCP.payments], 8640),
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
        prod("active", [LLM.anthropic], []),
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
        prod("active", [LLM.anthropic], [MCP.jira], 1440),
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
        prod("active", [LLM.openai], [MCP.jira], 57600),
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
        dev("in-progress", [LLM.anthropic], [MCP.github], 30),
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
        prod("failed", [LLM.azure], [MCP.hr], 540),
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
    // Unregistered: shadow agents that Shadow Agent Discovery found outside Agent Manager.
    {
      name: "openai-support-assistant",
      displayName: "Support assistant (OpenAI)",
      registration: "unregistered",
      discovery: {
        findingId: "f-openai-1",
        provider: "openai",
        providerDisplayName: "OpenAI",
        scope: "proj_customer-success",
        ownerHint: { name: "Dilini Rajapaksha", email: "dilini@example.com" },
        firstSeenAt: minutesAgo(60 * 24 * 20),
        lastSeenAt: minutesAgo(60 * 6),
        present: true,
        onboarded: false,
      },
      projectName: "",
      projectDisplayName: "",
      hosting: "external",
      interface: null,
      createdBy: null,
      labels: {},
      environments: [],
      createdAt: minutesAgo(60 * 24 * 20),
    },
    {
      name: "claims-runtime",
      displayName: "claims-runtime",
      registration: "unregistered",
      discovery: {
        findingId: "f-bedrock-1",
        provider: "bedrock",
        providerDisplayName: "Amazon Bedrock",
        scope: "us-east-1",
        // Bedrock's listing doesn't say who created it.
        ownerHint: {},
        firstSeenAt: minutesAgo(60 * 24 * 3),
        lastSeenAt: minutesAgo(60 * 6),
        present: true,
        onboarded: false,
      },
      projectName: "",
      projectDisplayName: "",
      hosting: "external",
      interface: null,
      createdBy: null,
      labels: {},
      environments: [],
      createdAt: minutesAgo(60 * 24 * 3),
    },
    {
      name: "campaign-writer",
      displayName: "Campaign writer",
      registration: "unregistered",
      discovery: {
        findingId: "f-google-1",
        provider: "google",
        providerDisplayName: "Google Vertex AI",
        scope: "projects/marketing-prod/locations/europe-west1",
        ownerHint: { email: "marketing-ops@example.com" },
        firstSeenAt: minutesAgo(60 * 24 * 45),
        lastSeenAt: minutesAgo(60 * 30),
        present: true,
        onboarded: false,
      },
      projectName: "",
      projectDisplayName: "",
      hosting: "external",
      interface: null,
      createdBy: null,
      labels: {},
      environments: [],
      createdAt: minutesAgo(60 * 24 * 45),
    },
    // Two findings the registry must not list: one onboarded (it is now the
    // registered external agent "Invoice reader"), and one its platform no longer lists.
    {
      name: "invoice-reader-openai",
      displayName: "Invoice reader (OpenAI)",
      registration: "unregistered",
      discovery: {
        findingId: "f-openai-2",
        provider: "openai",
        providerDisplayName: "OpenAI",
        scope: "proj_finance",
        ownerHint: { email: "finance-dev@example.com" },
        firstSeenAt: minutesAgo(60 * 24 * 210),
        lastSeenAt: minutesAgo(60 * 4),
        present: true,
        onboarded: true,
      },
      projectName: "",
      projectDisplayName: "",
      hosting: "external",
      interface: null,
      createdBy: null,
      labels: {},
      environments: [],
      createdAt: minutesAgo(60 * 24 * 210),
    },
    {
      name: "old-faq-bot",
      displayName: "Old FAQ bot",
      registration: "unregistered",
      discovery: {
        findingId: "f-google-2",
        provider: "google",
        providerDisplayName: "Google Vertex AI",
        scope: "projects/support-sandbox/locations/us-central1",
        ownerHint: {},
        firstSeenAt: minutesAgo(60 * 24 * 90),
        lastSeenAt: minutesAgo(60 * 24 * 12),
        present: false,
        onboarded: false,
      },
      projectName: "",
      projectDisplayName: "",
      hosting: "external",
      interface: null,
      createdBy: null,
      labels: {},
      environments: [],
      createdAt: minutesAgo(60 * 24 * 90),
    },
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

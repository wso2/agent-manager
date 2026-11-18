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
  ListAgentRegistryPathParams,
} from "@agent-management-platform/types";

// SAMPLE DATA. The registry API (GET /orgs/{org}/agents with registry fields,
// issue #78) does not exist yet. Unregistered agents will come from Shadow Agent
// Discovery (issue #71). When it does, replace the body of
// listAgentRegistry with an httpGET call and delete the sample data below.

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
): AgentRegistryEnvironment {
  return {
    name: "development",
    displayName: "Development",
    isProduction: false,
    deploymentStatus: status,
    llmProviders,
    mcpServers,
  };
}

function prod(
  status: AgentRegistryEnvironment["deploymentStatus"],
  llmProviders: AgentRegistryResourceRef[],
  mcpServers: AgentRegistryResourceRef[],
): AgentRegistryEnvironment {
  return {
    name: "production",
    displayName: "Production",
    isProduction: true,
    deploymentStatus: status,
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
      owner: { type: "group", id: "g-travel", displayName: "Travel platform team" },
      lifecycleState: "active",
      visibility: "org",
      labels: { team: "travel", "data-class": "confidential" },
      environments: [
        dev("active", [LLM.openai], [MCP.hotels, MCP.payments]),
        prod("active", [LLM.openai], [MCP.hotels, MCP.payments]),
      ],
      lastActivityAt: minutesAgo(12),
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
      owner: { type: "user", id: "u-nimal", displayName: "Nimal Perera" },
      lifecycleState: "active",
      visibility: "project",
      labels: { team: "finance" },
      environments: [
        dev("active", [LLM.azure], [MCP.payments]),
        prod("active", [LLM.azure], [MCP.payments]),
      ],
      lastActivityAt: minutesAgo(60 * 3),
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
      owner: null,
      lifecycleState: "active",
      visibility: "project",
      labels: {},
      environments: [
        dev("active", [LLM.anthropic], []),
        prod("active", [LLM.anthropic], []),
      ],
      lastActivityAt: minutesAgo(60 * 24 * 41),
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
      owner: { type: "group", id: "g-support", displayName: "Support engineering" },
      lifecycleState: "active",
      visibility: "org",
      labels: { team: "support", channel: "web" },
      environments: [
        dev("active", [LLM.anthropic], [MCP.jira]),
        prod("active", [LLM.anthropic], [MCP.jira]),
      ],
      lastActivityAt: minutesAgo(2),
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
      owner: { type: "user", id: "u-amaya", displayName: "Amaya Silva" },
      lifecycleState: "deprecated",
      visibility: "project",
      labels: { team: "support" },
      environments: [
        dev("suspended", [LLM.openai], [MCP.jira]),
        prod("active", [LLM.openai], [MCP.jira]),
      ],
      lastActivityAt: minutesAgo(60 * 26),
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
      owner: { type: "group", id: "g-support", displayName: "Support engineering" },
      lifecycleState: "retired",
      visibility: "project",
      labels: { team: "support" },
      environments: [
        dev("not-deployed", [], []),
        prod("suspended", [LLM.openai], []),
      ],
      lastActivityAt: minutesAgo(60 * 24 * 95),
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
      owner: { type: "user", id: "u-kasun", displayName: "Kasun Fernando" },
      lifecycleState: "draft",
      visibility: "project",
      labels: { team: "devex" },
      environments: [
        dev("in-progress", [LLM.anthropic], [MCP.github]),
        prod("not-deployed", [], []),
      ],
      lastActivityAt: null,
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
      owner: null,
      lifecycleState: "active",
      visibility: "org",
      labels: { team: "devex" },
      environments: [
        dev("active", [LLM.openai], [MCP.github, MCP.jira]),
        prod("active", [LLM.openai], [MCP.github, MCP.jira]),
      ],
      lastActivityAt: minutesAgo(60 * 8),
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
      owner: { type: "group", id: "g-hr", displayName: "People systems" },
      lifecycleState: "active",
      visibility: "project",
      labels: { team: "hr", "data-class": "personal" },
      environments: [
        dev("active", [LLM.azure], [MCP.hr]),
        prod("failed", [LLM.azure], [MCP.hr]),
      ],
      lastActivityAt: minutesAgo(45),
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
      owner: null,
      lifecycleState: "draft",
      visibility: "project",
      labels: { team: "hr", "data-class": "personal" },
      environments: [
        dev("failed", [LLM.azure], [MCP.hr, MCP.payments]),
        prod("not-deployed", [], []),
      ],
      lastActivityAt: null,
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
        ownerHint: "dilini@example.com",
        firstSeenAt: minutesAgo(60 * 24 * 20),
        lastSeenAt: minutesAgo(60 * 6),
      },
      projectName: "",
      projectDisplayName: "",
      hosting: "external",
      interface: null,
      owner: null,
      lifecycleState: null,
      visibility: "project",
      labels: {},
      environments: [],
      lastActivityAt: minutesAgo(60 * 6),
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
        firstSeenAt: minutesAgo(60 * 24 * 3),
        lastSeenAt: minutesAgo(60 * 6),
      },
      projectName: "",
      projectDisplayName: "",
      hosting: "external",
      interface: null,
      owner: null,
      lifecycleState: null,
      visibility: "project",
      labels: {},
      environments: [],
      lastActivityAt: minutesAgo(60 * 6),
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
        scope: "marketing-prod / europe-west1",
        ownerHint: "Marketing ops",
        firstSeenAt: minutesAgo(60 * 24 * 45),
        lastSeenAt: minutesAgo(60 * 30),
      },
      projectName: "",
      projectDisplayName: "",
      hosting: "external",
      interface: null,
      owner: null,
      lifecycleState: null,
      visibility: "project",
      labels: {},
      environments: [],
      lastActivityAt: minutesAgo(60 * 30),
      createdAt: minutesAgo(60 * 24 * 45),
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

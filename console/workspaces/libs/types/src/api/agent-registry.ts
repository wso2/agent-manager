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

import type { AgentThunderStatus } from './agents';
import type { AgentPathParams, OrgPathParams } from './common';
import type { MonitorStatus } from './monitors';

// Org-level agent registry (product-management issue #78). The list reads
// GET /orgs/{org}/agents?view=registry (registered agents) and the Shadow Agent Discovery
// findings (unregistered agents). A registered agent's overview reads GET agent with
// ?view=registry; an unregistered agent's reads its finding.

/** Where the agent runs: built and deployed by the platform, or registered from outside. */
export type AgentRegistryHosting = 'platform' | 'external';

/** How callers talk to the agent. Same values as the agent sub-type. */
export type AgentRegistryInterface = 'a2a-agent' | 'custom-api' | 'chat-api';

/** Deployment status of the agent in one environment. */
export type AgentRegistryDeploymentStatus =
  | 'not-deployed'
  | 'in-progress'
  | 'active'
  | 'failed'
  | 'suspended';

/**
 * The user who created the agent, which the registry shows as its owner until owners
 * exist. Same data as AgentResponse.createdBy (resolved from the AgentID record).
 */
export interface AgentRegistryCreatedBy {
  id: string;
  displayName: string;
}

/** A resource the agent can reach (an LLM provider). */
export interface AgentRegistryResourceRef {
  id: string;
  displayName: string;
}

/** An MCP server the agent can reach, and the tools it offers (from capability discovery). */
export interface AgentRegistryMcpServerRef extends AgentRegistryResourceRef {
  tools: string[];
}

/** An evaluation monitor on the agent in one environment (MonitorResponse). */
export interface AgentRegistryMonitorRef {
  name: string;
  displayName: string;
  status: MonitorStatus;
}

/**
 * The agent in one environment of its project's pipeline, as the registry list returns it:
 * its deployment and the LLM providers and MCP servers it is configured with there.
 */
export interface AgentRegistryEnvironment {
  name: string;
  displayName: string;
  isProduction: boolean;
  deploymentStatus: AgentRegistryDeploymentStatus;
  /** Null when the agent was never deployed here. Platform agents only. */
  lastDeployedAt: string | null;
  llmProviders: AgentRegistryResourceRef[];
  mcpServers: AgentRegistryResourceRef[];
}

/**
 * The agent in one environment on its overview page: the list's fields, and its governance
 * posture read from existing config (AgentID, guardrails, MCP tools, monitors).
 */
export interface AgentRegistryEnvironmentDetail extends AgentRegistryEnvironment {
  /** The agent's AgentID in this environment; null when none was requested. */
  agentIdStatus: AgentThunderStatus | null;
  /** Names of the guardrail policies on the agent's LLM provider config here. */
  guardrails: string[];
  mcpServers: AgentRegistryMcpServerRef[];
  monitors: AgentRegistryMonitorRef[];
  /** Who may call the agent here: the security on its endpoint (agent configurations). */
  inboundSecurity: { apiKey: boolean; oauth: boolean };
  /** The A2A card stored for this environment (#65); A2A agents only, once fetched. */
  a2aCard?: { skills: AgentRegistryA2ASkill[] };
}

/** One skill from an A2A agent card. */
export interface AgentRegistryA2ASkill {
  id: string;
  name: string;
  description?: string;
  tags?: string[];
}

/**
 * Registered: created or registered in a project of Agent Manager.
 * Unregistered: a shadow agent, found by Shadow Agent Discovery (issue #71) on a
 * platform outside Agent Manager and not onboarded yet.
 */
export type AgentRegistryRegistration = 'registered' | 'unregistered';

/** Who created a shadow agent, as its platform's listing reports it. Same as ShadowOwnerHint. */
export interface AgentRegistryOwnerHint {
  name?: string;
  email?: string;
  /** Where the platform says the creator came from, when it says. */
  source?: string;
}

/**
 * platform_agent: an agent object on the platform; runtime: a hosted agent runtime;
 * code_agent: an agent found in source code (ShadowFindingKind).
 */
export type AgentRegistryFindingKind = 'platform_agent' | 'runtime' | 'code_agent';

/** One connection that found the agent (ShadowFindingSource). */
export interface AgentRegistryFindingSource {
  findingId: string;
  connectionId: string;
  connectionName: string;
  present: boolean;
}

/**
 * The fields of a Shadow Agent Discovery finding the registry uses. Names follow
 * ShadowFindingSummary on the shadow-agent-registry branch (findingId is its id).
 */
export interface AgentRegistryDiscovery {
  findingId: string;
  /** The platform, e.g. "openai" or "bedrock". */
  provider: string;
  /** Not on the finding: the platform's name from the discovery provider list. */
  providerDisplayName: string;
  /** Where the agent lives at the platform (project, workspace, region or location). */
  scope: string;
  /** Who created it, only when the platform's listing says so; may be empty. */
  ownerHint: AgentRegistryOwnerHint;
  /** When a scan first found the agent. */
  firstSeenAt: string;
  /** The latest scan that still found it. */
  lastSeenAt: string;
  /** False once a complete listing of its connection no longer returns the agent. */
  present: boolean;
  /** True once it is onboarded as an external agent, which then lists it as registered. */
  onboarded: boolean;
  /** The connection that saw it last. */
  connectionName: string;
  kind: AgentRegistryFindingKind;
  /** The platform's own ID of the agent (agent ID, ARN or resource name). */
  externalId: string;
  /** The agent's state as its platform reports it. */
  state: string;
  providerCreatedAt?: string;
  foundBy: AgentRegistryFindingSource[];
}

/** One row of the registry: one agent, from any project, or one found outside Agent Manager. */
export interface AgentRegistryEntry {
  name: string;
  displayName: string;
  description?: string;
  registration: AgentRegistryRegistration;
  /** Set for unregistered agents only. */
  discovery?: AgentRegistryDiscovery;
  /** Empty for unregistered agents, which belong to no project. */
  projectName: string;
  projectDisplayName: string;
  hosting: AgentRegistryHosting;
  /** Null for unregistered agents: how callers reach them is not known. */
  interface: AgentRegistryInterface | null;
  kindName?: string;
  kindVersion?: string;
  /** Null when it could not be resolved (older agents). Always null for unregistered agents. */
  createdBy: AgentRegistryCreatedBy | null;
  labels: Record<string, string>;
  /** Empty for unregistered agents. */
  environments: AgentRegistryEnvironment[];
  createdAt: string;
}

/** One agent on its org-level overview page: the list's record, with governance per environment. */
export interface AgentRegistryEntryDetail extends Omit<AgentRegistryEntry, 'environments'> {
  environments: AgentRegistryEnvironmentDetail[];
}

export interface AgentRegistryListResponse {
  agents: AgentRegistryEntry[];
}

// The registry record in GET /orgs/{org}/agents?view=registry (each agent's) and in
// GET /orgs/{org}/projects/{proj}/agents/{agent}?view=registry (the agent's, with governance).

/** An LLM provider or MCP server; MCP servers carry their tools on the single-agent view. */
export interface AgentRegistryApiResourceRef extends AgentRegistryResourceRef {
  tools?: string[];
}

/** The agent's governance in one environment; on the single-agent view only. */
export interface AgentRegistryApiGovernance {
  /** Absent when no AgentID was requested here. */
  agentIdStatus?: AgentThunderStatus;
  guardrails: string[];
  monitors: AgentRegistryMonitorRef[];
  inboundSecurity: { apiKey: boolean; oauth: boolean };
  /** Absent when no A2A card is stored here. */
  a2aSkills?: AgentRegistryA2ASkill[];
}

export interface AgentRegistryApiEnvironment {
  name: string;
  displayName: string;
  isProduction: boolean;
  deploymentStatus: AgentRegistryDeploymentStatus;
  /** Absent when the agent was never deployed here. */
  lastDeployedAt?: string;
  llmProviders: AgentRegistryApiResourceRef[];
  mcpServers: AgentRegistryApiResourceRef[];
  governance?: AgentRegistryApiGovernance;
}

export interface AgentRegistryApiRecord {
  description?: string;
  hosting: AgentRegistryHosting;
  interface?: AgentRegistryInterface;
  kindName?: string;
  kindVersion?: string;
  /**
   * Absent when no creator was recorded. displayName is empty when the user could not be
   * looked up.
   */
  createdBy?: AgentRegistryCreatedBy;
  labels: Record<string, string>;
  createdAt: string;
  environments: AgentRegistryApiEnvironment[];
}

export interface AgentRegistryApiAgent {
  name: string;
  displayName: string;
  projectName: string;
  projectDisplayName: string;
  registry: AgentRegistryApiRecord;
}

export interface AgentRegistryApiListResponse {
  agents: AgentRegistryApiAgent[];
}

export type ListAgentRegistryPathParams = OrgPathParams;

/** One registered agent's registry record, for the org-level agent overview page. */
export type GetAgentRegistryEntryPathParams = AgentPathParams;

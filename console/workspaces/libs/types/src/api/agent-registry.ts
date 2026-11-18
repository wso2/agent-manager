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

import type { OrgPathParams } from './common';

// Org-level agent registry (product-management issue #78). The backend API does
// not exist yet; these shapes are what the console expects it to return.

/** Where the agent runs: built and deployed by the platform, or registered from outside. */
export type AgentRegistryHosting = 'platform' | 'external';

/** How callers talk to the agent. Same values as the agent sub-type. */
export type AgentRegistryInterface = 'a2a-agent' | 'custom-api' | 'chat-api';

/** The agent's place in its life, set by its owner. */
export type AgentRegistryLifecycleState = 'draft' | 'active' | 'deprecated' | 'retired';

/** Who can see the agent: its project's users only, or everyone in the organization. */
export type AgentRegistryVisibility = 'project' | 'org';

/** Deployment status of the agent in one environment. */
export type AgentRegistryDeploymentStatus =
  | 'not-deployed'
  | 'in-progress'
  | 'active'
  | 'failed'
  | 'suspended';

/** The user or group that owns the agent. */
export interface AgentRegistryOwner {
  type: 'user' | 'group';
  id: string;
  displayName: string;
}

/** A resource the agent can reach (an LLM provider or an MCP server). */
export interface AgentRegistryResourceRef {
  id: string;
  displayName: string;
}

/** The agent in one environment: its status and what it can reach there. */
export interface AgentRegistryEnvironment {
  name: string;
  displayName: string;
  isProduction: boolean;
  deploymentStatus: AgentRegistryDeploymentStatus;
  llmProviders: AgentRegistryResourceRef[];
  mcpServers: AgentRegistryResourceRef[];
}

/** One row of the registry: one agent, from any project. */
export interface AgentRegistryEntry {
  name: string;
  displayName: string;
  description?: string;
  projectName: string;
  projectDisplayName: string;
  hosting: AgentRegistryHosting;
  interface: AgentRegistryInterface;
  kindName?: string;
  kindVersion?: string;
  /** Null when no owner has been set. */
  owner: AgentRegistryOwner | null;
  lifecycleState: AgentRegistryLifecycleState;
  visibility: AgentRegistryVisibility;
  labels: Record<string, string>;
  environments: AgentRegistryEnvironment[];
  /** Time of the agent's latest trace. Null when it has never been called. */
  lastActivityAt: string | null;
  createdAt: string;
}

export interface AgentRegistryListResponse {
  agents: AgentRegistryEntry[];
}

export type ListAgentRegistryPathParams = OrgPathParams;

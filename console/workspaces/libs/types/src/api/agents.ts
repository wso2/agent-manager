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

import { type AgentPathParams, type Build, type Configurations, type ListQuery, type OrgPathParams, type OrgProjPathParams, type PaginationMeta, type RepositoryConfig } from './common';
import type { EnvProviderConfiguration, EnvironmentVariableConfig } from './agent-model-configs';
import type { ThunderGroup, ThunderRole } from './identities';

export interface ModelConfigRequest {
  providerName: string;
  configuration?: EnvProviderConfiguration;
  environmentVariables?: EnvironmentVariableConfig[];
}

export interface MCPConfigRequest {
  proxyName: string;
  environmentVariables?: EnvironmentVariableConfig[];
}

// Requests
interface AgentRequestBase {
  name: string;
  displayName: string;
  description?: string;
  provisioning: Provisioning;
  agentType?: AgentType;
  build?: Build;
  configurations?: Configurations;
  inputInterface?: InputInterface;
  modelConfig?: ModelConfigRequest[];
  mcpConfig?: MCPConfigRequest[];
  labels?: Record<string, string>;
}

interface UpdateAgentBasicInfoRequest {
  displayName: string;
  description?: string;
  /** Omit to leave labels unchanged; send {} to clear all labels. */
  labels?: Record<string, string>;
}

interface UpdateAgentBuildParametersRequest {
  provisioning: Provisioning;
  agentType?: AgentType;
  build?: Build;
  configurations?: Configurations;
  inputInterface?: InputInterface;
}

export type CreateAgentRequest = AgentRequestBase;
export type UpdateAgentRequest = UpdateAgentBasicInfoRequest;
export type { UpdateAgentBasicInfoRequest, UpdateAgentBuildParametersRequest };

export type InputInterfaceType = 'DEFAULT' | 'CUSTOM' | 'A2A';

export interface InputInterface {
  type: string; // Always "HTTP" for now
  port?: number;
  schema?: {
    path: string;
  };
  basePath?: string;
}

export interface AgentType {
  type: string;
  subType: string;
}

export type ProvisioningType = 'internal' | 'external';

export interface ProvisioningAgentKind {
  name: string;
  version: string;
}

export interface Provisioning {
  type: ProvisioningType;
  repository?: RepositoryConfig;
  agentKind?: ProvisioningAgentKind;
}

/**
 * Creator of an agent, resolved best-effort by the control plane — absent for
 * agents created before this was tracked, or when the user can't be resolved.
 */
export interface AgentCreatedBy {
  /** User ID captured at creation time. */
  id: string;
  /** Display name/username, omitted when the user could not be resolved. */
  display?: string;
}

export interface AgentResponse {
  name: string;
  displayName: string;
  description: string;
  createdAt: string; // ISO date-time
  projectName: string;
  status?: string;
  provisioning: Provisioning;
  agentType?: AgentType;
  build?: Build;
  configurations?: Configurations;
  inputInterface?: InputInterface;
  uuid?: string;
  kindName?: string;
  kindVersion?: string;
  labels?: Record<string, string>;
  createdBy?: AgentCreatedBy;
}

export interface AgentListResponse extends PaginationMeta {
  agents: AgentResponse[];
}

// Lightweight org-wide listing: every agent across all projects, unpaginated.
// Includes the owning project's name/displayName so consumers can resolve
// both without a separate project lookup.
export interface AgentSummary {
  name: string;
  displayName: string;
  projectName: string;
  projectDisplayName: string;
}

export interface AgentSummaryListResponse {
  agents: AgentSummary[];
}

export type ListOrgAgentsPathParams = OrgPathParams;

// Path/Query helpers
export type ListAgentsPathParams = OrgProjPathParams;
export type CreateAgentPathParams = OrgProjPathParams;
export type GetAgentPathParams = AgentPathParams;
export type DeleteAgentPathParams = AgentPathParams;
export type UpdateAgentPathParams = AgentPathParams;
export type UpdateAgentBasicInfoPathParams = AgentPathParams;
export type UpdateAgentBuildParametersPathParams = AgentPathParams;
/** `label` entries are `key:value` selectors; repeat for AND semantics. */
export type ListAgentsQuery = ListQuery & { label?: string[] };

// Agent Token
export interface TokenRequest {
  expires_in?: string; // Go duration format (e.g., "720h" for 30 days, "8760h" for 1 year)
}

export interface TokenResponse {
  token: string;
  expires_at: number; // Unix timestamp
  issued_at: number; // Unix timestamp
  token_type: string; // "Bearer"
}

export type GenerateAgentTokenPathParams = AgentPathParams;

export interface GenerateAgentTokenQuery {
  environment?: string;
}

// --- Agent identity: roles/groups (read-only) ---

export type GetAgentRolesPathParams = AgentPathParams;
export type GetAgentGroupsPathParams = AgentPathParams;

export interface GetAgentRolesQuery {
  environment: string;
}

export interface GetAgentGroupsQuery {
  environment: string;
}

export interface AgentRolesResponse {
  roles: ThunderRole[];
}

export interface AgentGroupsResponse {
  groups: ThunderGroup[];
}

// --- Agent identity: AgentID lifecycle (per environment) ---

export type AgentThunderStatus = 'pending' | 'in_progress' | 'completed' | 'failed';

// One environment's AgentID binding. Never includes a secret — for an
// externally hosted agent, use RegenerateAgentIdentitySecret to obtain one.
export interface AgentIdentityEnvironmentView {
  environmentName: string;
  provisioningType: ProvisioningType;
  status: AgentThunderStatus;
  agentId?: string;
  clientId?: string;
  lastError?: string;
  requestedBy?: string;
}

export interface AgentIdentityActionRequest {
  environment: string;
}

export interface AgentRegenerateSecretResponse {
  environmentName: string;
  provisioningType: ProvisioningType;
  clientId: string;
  clientSecret: string;
  status: string;
}

// Never includes clientSecret — revoke turns access off rather than rotating it.
export interface AgentRevokeSecretResponse {
  environmentName: string;
  clientId: string;
  status: string;
}

export type GetAgentIdentityPathParams = AgentPathParams;
export interface GetAgentIdentityQuery {
  environment?: string;
}

export type ProvisionAgentIdentityPathParams = AgentPathParams;
export interface ProvisionAgentIdentityQuery {
  environment: string;
}

export type RegenerateAgentIdentitySecretPathParams = AgentPathParams;

export type RetryAgentIdentityProvisioningPathParams = AgentPathParams;

export type RevokeAgentIdentitySecretPathParams = AgentPathParams;
export interface RevokeAgentIdentitySecretQuery {
  environment: string;
}


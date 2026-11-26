/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { type AgentPathParams, type AgentCardCorsConfig, type CorsConfig, type OAuthConfig, type FileMount } from './common';
import type { GatewaySpec } from './gateways';
import type { HealthChecks } from './agents';

export interface UpdateAgentDeploySettingsRequest {
  /** Ignored: the settings apply to the default environment. */
  environmentName?: string;
  enableAutoInstrumentation?: boolean;
  /**
   * AMP instrumentation version to pin for this Python buildpack agent in this
   * environment. Omit to keep the currently-pinned version.
   */
  instrumentationVersion?: string;
  enableApiKeySecurity?: boolean;
  enableOAuthSecurity?: boolean;
  corsConfig?: CorsConfig;
  agentCardCorsConfig?: AgentCardCorsConfig;
  oauthConfig?: OAuthConfig;
  resilienceTimeoutSeconds?: number;
}

export type UpdateAgentDeploySettingsPathParams = AgentPathParams;

export type DeploymentVisibility = 'Public' | 'Private' | 'Internal';

export interface DeploymentEndpoint {
  name: string;
  url: string;
  visibility: DeploymentVisibility;
}

export interface EnvironmentObject {
  name: string;
  displayName: string;
}

export interface PromotionTargetEnvironment {
  name: string;
  displayName: string;
}

export interface DeploymentDetailsResponse {
  imageId: string;
  /** Agent Kind version running in this environment, resolved from the deployed
   *  image. Absent for source-built agents and unmatched images. */
  kindVersion?: string;
  status: string;
  lastDeployed: string; // ISO date-time
  endpoints: DeploymentEndpoint[];
  environmentDisplayName?: string;
  promotionTargetEnvironment?: PromotionTargetEnvironment;
  /** Health checks this environment runs: those of the build it runs. Absent means the defaults. */
  healthChecks?: HealthChecks;
}

export type DeploymentListResponse = Record<string, DeploymentDetailsResponse>;

export interface ConfigurationItem {
  key: string;
  value: string;
  isSensitive?: boolean;
  secretRef?: string;
  isSystem?: boolean;
}

export interface ConfigurationData {
  env: ConfigurationItem[];
  files?: FileMount[];
}

export interface ConfigurationResponse {
  projectName: string;
  agentName: string;
  environment: string;
  enableAutoInstrumentation?: boolean;
  instrumentationVersion?: string;
  enableApiKeySecurity?: boolean;
  enableOAuthSecurity?: boolean;
  corsConfig?: CorsConfig;
  agentCardCorsConfig?: AgentCardCorsConfig;
  oauthConfig?: OAuthConfig;
  /**
   * Max duration (seconds) the gateway keeps a response open between the agent and the
   * client before cutting it off, for this agent's endpoint in this environment.
   */
  resilienceTimeoutSeconds?: number;
  /** The agent's LLM and MCP bindings and whether the gateway has provisioned them. */
  bindings: AgentBinding[];
}

export type AgentBindingState = 'pending' | 'ready' | 'failed' | 'removing';

export interface AgentBinding {
  kind: 'llm' | 'mcp';
  /** Index in the create request's modelConfig or mcpConfig list. */
  position: number;
  /** The LLM provider or MCP proxy handle. */
  target: string;
  state: AgentBindingState;
  lastError?: string;
  configId?: string;
}

export interface Environment {
  name: string;
  dataplaneRef: string;
  displayName?: string;
  isProduction: boolean;
  dnsPrefix?: string;
  gateway?: GatewaySpec;
  createdAt: string; // ISO date-time
  id?: string;
  isolationTier?: string;
}

export type EnvironmentListResponse = Environment[];

export type ListAgentDeploymentsPathParams = AgentPathParams;
export type GetAgentConfigurationsPathParams = AgentPathParams;
export type ListEnvironmentsPathParams = { orgName: string | undefined };

export interface EnvironmentQuery {
  environment: string;
}

export type GetEnvironmentPathParams = {
  orgName: string | undefined;
  envName: string | undefined;
};

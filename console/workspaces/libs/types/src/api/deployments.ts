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

import { type AgentPathParams, type CorsConfig, type OAuthConfig, type EnvironmentVariable, type FileMount, type EndpointSchema, type OrgProjPathParams, type PaginationMeta, type ListQuery } from './common';
import type { GatewaySpec } from './gateways';

// Requests
export interface DeployAgentRequest {
  imageId: string;
  env?: EnvironmentVariable[];
  files?: FileMount[];
  enableAutoInstrumentation?: boolean;
  instrumentationVersion?: string;
  enableApiKeySecurity?: boolean;
  enableOAuthSecurity?: boolean;
  corsConfig?: CorsConfig;
  oauthConfig?: OAuthConfig;
}

export interface UpdateAgentDeploySettingsRequest {
  environmentName: string;
  enableAutoInstrumentation?: boolean;
  /**
   * AMP instrumentation version to pin for this Python buildpack agent in this
   * environment. Omit to keep the currently-pinned version.
   */
  instrumentationVersion?: string;
  enableApiKeySecurity?: boolean;
  enableOAuthSecurity?: boolean;
  corsConfig?: CorsConfig;
  oauthConfig?: OAuthConfig;
  resilienceTimeoutSeconds?: number;
}

export type UpdateAgentDeploySettingsPathParams = AgentPathParams;

export interface UpdateAgentConfigurationsRequest {
  environmentName: string;
  env?: EnvironmentVariable[];
  files?: FileMount[];
}

export type UpdateAgentConfigurationsPathParams = AgentPathParams;

export interface RegenerateTracingTokenRequest {
  environmentName: string;
  /** Go duration string (e.g. "2160h"). Omit to use the server-configured default. */
  expiresIn?: string;
}

export type RegenerateTracingTokenPathParams = AgentPathParams;

export interface RegenerateTracingTokenResponse {
  environmentName: string;
  expiresAt: number;
  rotatedAt: number;
}

// Responses
export interface DeploymentResponse {
  agentName: string;
  projectName: string;
  imageId: string;
  environment: string;
}

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
  status: string;
  lastDeployed: string; // ISO date-time
  endpoints: DeploymentEndpoint[];
  environmentDisplayName?: string;
  promotionTargetEnvironment?: PromotionTargetEnvironment;
}

export type DeploymentListResponse = Record<string, DeploymentDetailsResponse>;

export interface EndpointConfiguration {
  url: string;
  endpointName: string;
  schema: EndpointSchema;
  visibility: string;
}

export type EndpointsResponse = Record<string, EndpointConfiguration>;

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
  oauthConfig?: OAuthConfig;
  /**
   * Max duration (seconds) the gateway keeps a response open between the agent and the
   * client before cutting it off, for this agent's endpoint in this environment.
   */
  resilienceTimeoutSeconds?: number;
  configurations: ConfigurationData;
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

export interface DataPlane {
  name: string;
  displayName: string;
  description: string;
  orgName: string;
  createdAt: string; // ISO date-time
}

export type DataPlaneListResponse = DataPlane[];

export interface TargetEnvironmentRef {
  name: string;
}

export interface PromotionPath {
  sourceEnvironmentRef: string;
  targetEnvironmentRefs: TargetEnvironmentRef[];
}

export interface DeploymentPipelineResponse {
  name: string;
  displayName: string;
  description: string;
  orgName: string;
  createdAt: string; // ISO date-time
  promotionPaths: PromotionPath[];
}

export interface DeploymentPipelineListResponse extends PaginationMeta {
  deploymentPipelines: DeploymentPipelineResponse[];
}

// Path helpers
export type DeployAgentPathParams = AgentPathParams;
export type ListAgentDeploymentsPathParams = AgentPathParams;
export type GetAgentEndpointsPathParams = AgentPathParams;
export type GetAgentConfigurationsPathParams = AgentPathParams;
export type ListEnvironmentsPathParams = { orgName: string | undefined };
export type ListDataPlanesPathParams = { orgName: string | undefined };
export type ListDeploymentPipelinesPathParams = { orgName: string | undefined };
export type GetDeploymentPipelinePathParams = OrgProjPathParams;

// Query helpers
export interface EnvironmentQuery {
  environment: string;
}

export type ListDeploymentPipelinesQuery = ListQuery;

// Deployment State Types
export type DeploymentState = 'Active' | 'Undeploy';

export interface UpdateDeploymentStateRequest {
  environment: string;
  state: DeploymentState;
}

export interface UpdateDeploymentStateResponse {
  message: string;
  environment: string;
  state: DeploymentState;
}

export type UpdateDeploymentStatePathParams = AgentPathParams;

// Promote Agent
export interface PromoteAgentRequest {
  sourceEnvironment: string;
  targetEnvironment: string;
  useConfigFromSourceEnv?: boolean;
  env?: EnvironmentVariable[];
  files?: FileMount[];
  enableAutoInstrumentation?: boolean;
  /**
   * AMP instrumentation version to pin for this Python buildpack agent in the
   * target environment. Omit to inherit the currently-pinned version.
   */
  instrumentationVersion?: string;
  enableApiKeySecurity?: boolean;
  enableOAuthSecurity?: boolean;
  corsConfig?: CorsConfig;
  oauthConfig?: OAuthConfig;
}

export interface PromoteAgentResponse {
  agentName?: string;
  projectName?: string;
  sourceEnvironment?: string;
  targetEnvironment?: string;
}

export type PromoteAgentPathParams = AgentPathParams;

// Update Deployment Pipeline
export interface UpdateDeploymentPipelineRequest {
  displayName?: string;
  description?: string;
  promotionPaths: PromotionPath[];
}

// Update DeploymentPipeline (direct — by pipeline name, no project required)
export type UpdateOrgDeploymentPipelinePathParams = {
  orgName: string | undefined;
  pipelineName: string;
};

// Delete DeploymentPipeline (direct — by pipeline name)
export type DeleteDeploymentPipelinePathParams = {
  orgName: string | undefined;
  pipelineName: string;
};

// Create DeploymentPipeline
export interface CreateDeploymentPipelineRequest {
  displayName: string;
  description?: string;
  projectName?: string;
  promotionPaths: PromotionPath[];
}

export type CreateDeploymentPipelinePathParams = { orgName: string | undefined };

// Update Environment
export interface UpdateEnvironmentRequest {
  displayName?: string;
  description?: string;
  isProduction?: boolean;
}

export interface UpdateEnvironmentPathParams {
  orgName: string | undefined;
  envName: string | undefined;
}

// Get Environment
export type GetEnvironmentPathParams = {
  orgName: string | undefined;
  envName: string | undefined;
};

// Create Environment
export interface CreateEnvironmentRequest {
  name: string;
  displayName: string;
  description?: string;
  dataplaneRef: string;
  dnsPrefix: string;
  isProduction?: boolean;
}

export type CreateEnvironmentPathParams = { orgName: string | undefined };

// Delete Environment
export type DeleteEnvironmentPathParams = {
  orgName: string | undefined;
  envName: string | undefined;
};

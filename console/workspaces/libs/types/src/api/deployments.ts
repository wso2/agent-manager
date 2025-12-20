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

import { type AgentPathParams, type EnvironmentVariable, type EndpointSchema, type OrgProjPathParams, type PaginationMeta, type ListQuery } from './common';

// Requests
export interface DeployAgentRequest {
  imageId: string;
  env?: EnvironmentVariable[];
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
}

export interface ConfigurationResponse {
  projectName: string;
  agentName: string;
  environment: string;
  configurations: ConfigurationItem[];
}

export interface Environment {
  name: string;
  dataplaneRef: string;
  displayName?: string;
  isProduction: boolean;
  dnsPrefix?: string;
  createdAt: string; // ISO date-time
  uuid?: string;
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

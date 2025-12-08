import { type AgentPathParams, type EnvironmentVariable, type EndpointSchema, type OrgProjPathParams } from './common';

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
  sourceEnvironment: EnvironmentObject;
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
  namespace: string;
  displayName?: string;
  isProduction: boolean;
  dnsPrefix?: string;
  createdAt: string; // ISO date-time
}

export type EnvironmentListResponse = Environment[];

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

// Path helpers
export type DeployAgentPathParams = AgentPathParams;
export type ListAgentDeploymentsPathParams = AgentPathParams;
export type GetAgentEndpointsPathParams = AgentPathParams;
export type GetAgentConfigurationsPathParams = AgentPathParams;
export type ListEnvironmentsPathParams = { orgName: string };
export type GetDeploymentPipelinePathParams = OrgProjPathParams;

// Query helpers
export interface EnvironmentQuery {
  environment: string;
}

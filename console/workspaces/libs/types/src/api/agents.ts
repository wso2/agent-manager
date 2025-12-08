import { type AgentPathParams, type RuntimeConfiguration, type EndpointSpec, type ListQuery, type OrgProjPathParams, type PaginationMeta, type RepositoryConfig } from './common';

// Requests
export interface CreateAgentRequest {
  name: string;
  displayName: string;
  description?: string;
  provisioning: Provisioning;
  runtimeConfigs?: RuntimeConfiguration;
  inputInterface?: InputInterface;
}

export type InputInterfaceType = 'DEFAULT' | 'CUSTOM';

export interface InputInterface {
  type: string;
  customOpenAPISpec?: EndpointSpec;
}

export type ProvisioningType = 'internal' | 'external';

export interface Provisioning {
  type: ProvisioningType;
  repository?: RepositoryConfig;
}

export interface AgentResponse {
  name: string;
  displayName: string;
  description: string;
  createdAt: string; // ISO date-time
  projectName: string;
  status?: string;
  provisioning: Provisioning;
}

export interface AgentListResponse extends PaginationMeta {
  agents: AgentResponse[];
}

// Path/Query helpers
export type ListAgentsPathParams = OrgProjPathParams;
export type CreateAgentPathParams = OrgProjPathParams;
export type GetAgentPathParams = AgentPathParams;
export type DeleteAgentPathParams = AgentPathParams;
export type ListAgentsQuery = ListQuery;



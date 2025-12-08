// Shared/common API types reused across subjects

export interface PaginationMeta {
  total: number;
  limit: number;
  offset: number;
}

export interface ListQuery {
  limit?: number;
  offset?: number;
}

export interface RepositoryConfig {
  url: string;
  branch: string;
  appPath: string;
}

export interface EnvironmentVariable {
  key: string;
  value: string;
}

export interface RuntimeConfiguration {
  language: string;
  languageVersion: string;
  runCommand?: string;
  env?: EnvironmentVariable[];
}

export interface EndpointSchema {
  content: string;
}

export interface EndpointSpec {
  port: number; // 1 - 65535
  schema: EndpointSchema;
  basePath: string;
}

export interface ErrorResponse {
  message: string;
  description?: string;
  additionalData?: Record<string, unknown>;
}

// Common path parameters
export interface OrgProjPathParams {
  orgName: string;
  projName: string;
}

export interface AgentPathParams extends OrgProjPathParams {
  agentName: string;
}

export interface BuildPathParams extends AgentPathParams {
  buildName: string;
}

// Resource name generation
export type ResourceType = 'agent' | 'project';

export interface ResourceNameRequest {
  displayName: string;
  resourceType: ResourceType;
  projectName?: string; // Required if resourceType is 'agent'
}

export interface ResourceNameResponse {
  name: string;
  displayName: string;
  resourceType: ResourceType;
}

export interface GenerateResourceNamePathParams {
  orgName: string;
}



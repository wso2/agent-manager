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
export interface OrgPathParams {
  orgName: string | undefined;
}

export interface OrgProjPathParams extends OrgPathParams {
  projName: string | undefined;
}

export interface AgentPathParams extends OrgProjPathParams {
  agentName: string | undefined;
}

export interface BuildPathParams extends AgentPathParams {
  buildName: string | undefined;
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
  orgName: string | undefined;
}



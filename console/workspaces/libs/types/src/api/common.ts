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
  secretRef: string | null;
}

export interface EnvironmentVariable {
  key: string;
  value: string;
  isSensitive?: boolean;
  secretRef?: string;
  // System-managed entries are injected by the platform (e.g. LLM_PROVIDER_URL).
  // They are read-only and should be excluded from user-editable lists.
  isSystem?: boolean;
}

export interface FileMount {
  key: string;
  mountPath: string;
  value: string;
  isSensitive?: boolean;
  secretRef?: string;
}

export interface RuntimeConfiguration {
  language: string;
  languageVersion: string;
  runCommand?: string;
  env?: EnvironmentVariable[];
}

export interface RuntimeConfigurationWithoutEnv {
  language: string;
  languageVersion?: string;
  runCommand?: string;
}

export interface BuildpackConfig {
  language: string;
  languageVersion?: string;
  runCommand?: string;
}

export interface DockerConfig {
  dockerfilePath: string;
}

export interface BuildpackBuild {
  type: 'buildpack';
  buildpack: BuildpackConfig;
}

export interface DockerBuild {
  type: 'docker';
  docker: DockerConfig;
}

export type Build = BuildpackBuild | DockerBuild;

export interface CorsConfig {
  enabled?: boolean;
  allowOrigin?: string[];
  allowMethods?: string[];
  allowHeaders?: string[];
  allowCredentials?: boolean;
}

/** CORS for an A2A agent's public Agent Card route. Methods are always GET and OPTIONS. */
export interface AgentCardCorsConfig {
  /**
   * True when the card follows the agent's CORS. On a request it clears any override and the
   * other fields are ignored; on a response the other fields hold the effective values.
   */
  inherit?: boolean;
  enabled?: boolean;
  allowOrigin?: string[];
  allowHeaders?: string[];
  allowCredentials?: boolean;
}

export interface OAuthConfig {
  /** Issuer names referencing gateway-side key manager entries. Empty uses the platform default. */
  issuers?: string[];
  /** Accepted token audiences (aud claim). Empty disables audience validation. */
  audiences?: string[];
  /** Request header carrying the token. Defaults to "Authorization". */
  headerName?: string;
  /** Prefix before the token in the header value. Defaults to "Bearer". */
  authHeaderPrefix?: string;
  /** Forward the validated token header to the upstream service. Defaults to true. */
  forwardToken?: boolean;
}

export interface Configurations {
  env?: EnvironmentVariable[];
  files?: FileMount[];
  enableAutoInstrumentation?: boolean;
  instrumentationVersion?: string;
  enableApiKeySecurity?: boolean;
  enableOAuthSecurity?: boolean;
  corsConfig?: CorsConfig;
  oauthConfig?: OAuthConfig;
}

export interface EndpointSchema {
  content: string;
}

export interface SchemaPath {
  path: string;
}

export interface EndpointSpec {
  port: number; // 1 - 65535
  schema: SchemaPath;
  basePath: string;
}

export interface ErrorResponse {
  message: string;
  reason?: string;
  code: string;
  additionalData?: Record<string, unknown>;
}

// Masked, read-only view of a stored API key returned when listing an
// artifact's keys (LLM providers, MCP proxies). The plain key value is never
// returned — only the masked representation.
export interface APIKeyInfo {
  name: string;
  displayName?: string;
  maskedApiKey: string;
  status: string;
  createdAt: string;
  expiresAt?: string;
}

// Response shape for listing an artifact's API keys.
export interface ListAPIKeysResponse {
  keys: APIKeyInfo[];
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
export type ResourceType = 'agent' | 'project' | 'environment';

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



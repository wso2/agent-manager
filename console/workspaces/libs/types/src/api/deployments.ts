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

import { type AgentPathParams, type AgentCardCorsConfig, type CorsConfig, type OAuthConfig, type FileMount, type EndpointSchema } from './common';

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
  agentCardCorsConfig?: AgentCardCorsConfig;
  oauthConfig?: OAuthConfig;
  resilienceTimeoutSeconds?: number;
}

export type UpdateAgentDeploySettingsPathParams = AgentPathParams;

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
  agentCardCorsConfig?: AgentCardCorsConfig;
  oauthConfig?: OAuthConfig;
  /**
   * Max duration (seconds) the gateway keeps a response open between the agent and the
   * client before cutting it off, for this agent's endpoint in this environment.
   */
  resilienceTimeoutSeconds?: number;
  configurations: ConfigurationData;
}

export type GetAgentEndpointsPathParams = AgentPathParams;
export type GetAgentConfigurationsPathParams = AgentPathParams;

// Query helpers
export interface EnvironmentQuery {
  environment: string;
}

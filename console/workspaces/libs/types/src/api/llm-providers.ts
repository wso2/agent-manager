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

import type { OrgPathParams, OrgProjPathParams } from "./common";

export interface LLMProviderTemplateMetadata {
  description?: string;
  displayName?: string;
  logoUrl?: string;
  endpointUrl?: string;
  auth?: {
    type?: string;
    header?: string;
    valuePrefix?: string;
  };
  tags?: string[];
}

export interface LLMProviderTemplateAuth {
  type?: string;
  config?: Record<string, unknown>;
}

export interface LLMProviderTemplateResponse {
  id: string;
  name: string;
  orgName: string;
  metadata?: LLMProviderTemplateMetadata;
  auth?: LLMProviderTemplateAuth;
  createdAt: string;
  updatedAt: string;
}

export interface LLMProviderTemplateListResponse {
  templates: LLMProviderTemplateResponse[];
  total: number;
}

export interface CreateLLMProviderTemplateRequest {
  name: string;
  metadata?: LLMProviderTemplateMetadata;
  auth?: LLMProviderTemplateAuth;
}

export interface UpdateLLMProviderTemplateRequest {
  metadata?: LLMProviderTemplateMetadata;
  auth?: LLMProviderTemplateAuth;
}

export type ListLLMProviderTemplatesPathParams = OrgPathParams;
export type CreateLLMProviderTemplatePathParams = OrgPathParams;

export interface LLMProviderTemplatePathParams extends OrgPathParams {
  templateId: string | undefined;
}

export type GetLLMProviderTemplatePathParams = LLMProviderTemplatePathParams;
export type UpdateLLMProviderTemplatePathParams = LLMProviderTemplatePathParams;
export type DeleteLLMProviderTemplatePathParams = LLMProviderTemplatePathParams;

export interface LLMModel {
  id: string;
  name: string;
}

export interface LLMProviderConfig {
  endpoint?: string;
  models?: LLMModel[];
}

export interface LLMProviderResponse {
  id: string;
  name: string;
  displayName: string;
  orgName: string;
  templateId?: string;
  config?: LLMProviderConfig;
  createdAt: string;
  updatedAt: string;
}

export interface LLMProviderListResponse {
  providers: LLMProviderResponse[];
  total: number;
}

export interface CreateLLMProviderRequest {
  name: string;
  displayName: string;
  templateId?: string;
  config?: LLMProviderConfig;
}

export interface UpdateLLMProviderRequest {
  displayName?: string;
  config?: LLMProviderConfig;
}

export interface UpdateLLMProviderCatalogRequest {
  catalogEntry?: Record<string, unknown>;
}

export type ListLLMProvidersPathParams = OrgPathParams;
export type CreateLLMProviderPathParams = OrgPathParams;

export interface LLMProviderPathParams extends OrgPathParams {
  providerId: string | undefined;
}

export type GetLLMProviderPathParams = LLMProviderPathParams;
export type UpdateLLMProviderPathParams = LLMProviderPathParams;
export type DeleteLLMProviderPathParams = LLMProviderPathParams;
export type UpdateLLMProviderCatalogPathParams = LLMProviderPathParams;
export type ListLLMProviderProxiesPathParams = LLMProviderPathParams;

export interface LLMProxyConfig {
  endpoint?: string;
  rateLimiting?: Record<string, unknown>;
}

export interface LLMProxyResponse {
  id: string;
  name: string;
  displayName: string;
  orgName: string;
  projectName: string;
  providerId: string;
  config?: LLMProxyConfig;
  createdAt: string;
  updatedAt: string;
}

export interface LLMProxyListResponse {
  proxies: LLMProxyResponse[];
  total: number;
}

export interface CreateLLMProxyRequest {
  name: string;
  displayName: string;
  providerId: string;
  config?: LLMProxyConfig;
}

export interface UpdateLLMProxyRequest {
  displayName?: string;
  config?: LLMProxyConfig;
}

export type ListLLMProxiesPathParams = OrgProjPathParams;
export type CreateLLMProxyPathParams = OrgProjPathParams;

export interface LLMProxyPathParams extends OrgProjPathParams {
  proxyId: string | undefined;
}

export type GetLLMProxyPathParams = LLMProxyPathParams;
export type UpdateLLMProxyPathParams = LLMProxyPathParams;
export type DeleteLLMProxyPathParams = LLMProxyPathParams;

export interface LLMDeploymentResponse {
  id: string;
  providerId: string;
  environmentId: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface LLMDeploymentListResponse {
  deployments: LLMDeploymentResponse[];
  total: number;
}

export interface CreateLLMDeploymentRequest {
  environmentId: string;
}

export type ListLLMDeploymentsPathParams = LLMProviderPathParams;
export type CreateLLMDeploymentPathParams = LLMProviderPathParams;
export type UndeployLLMProviderPathParams = LLMProviderPathParams;
export type RestoreLLMDeploymentPathParams = LLMProviderPathParams;

export interface LLMDeploymentPathParams extends LLMProviderPathParams {
  deploymentId: string | undefined;
}

export type GetLLMDeploymentPathParams = LLMDeploymentPathParams;
export type DeleteLLMDeploymentPathParams = LLMDeploymentPathParams;


/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

export interface CatalogDeploymentSummary {
  gatewayId: string;
  gatewayName: string;
  environmentName?: string;
  status: string;
  deployedAt?: string;
  vhost?: string;
}

export interface CatalogLLMProviderEntry {
  uuid: string;
  handle: string;
  name: string;
  version: string;
  kind: string;
  inCatalog: boolean;
  status: string;
  template: string;
  createdAt: string;
  deployments?: CatalogDeploymentSummary[];
}

export interface ListCatalogLLMProvidersResponse {
  entries: CatalogLLMProviderEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface ListCatalogLLMProvidersParams {
  orgName: string | undefined;
}

export interface ListCatalogLLMProvidersQuery {
  kind?: "LlmProvider";
  limit?: number;
  offset?: number;
}

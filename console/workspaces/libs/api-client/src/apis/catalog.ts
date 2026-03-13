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

import { httpGET, SERVICE_BASE } from "../utils";

export interface CatalogLLMProviderEntry {
  uuid: string;
  handle: string;
  name: string;
  version: string;
  kind: string;
  inCatalog: boolean;
  status: string;
  template: string;
  createdAt?: string;
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

export async function listCatalogLLMProviders(
  params: ListCatalogLLMProvidersParams,
  query?: ListCatalogLLMProvidersQuery,
  getToken?: () => Promise<string>,
): Promise<ListCatalogLLMProvidersResponse> {
  const org = encodeURIComponent(params.orgName ?? "default");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    kind: "LlmProvider",
  };
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/catalog`, {
    token,
    searchParams,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

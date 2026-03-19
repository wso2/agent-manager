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

import { cloneDeep } from "lodash";
import { httpGET, httpPUT, SERVICE_BASE } from "../utils";
import type {
  AgentResourceConfigsResponse,
  GetAgentResourceConfigsPathParams,
  GetAgentResourceConfigsQuery,
  UpdateAgentResourceConfigsPathParams,
  UpdateAgentResourceConfigsQuery,
  UpdateAgentResourceConfigsRequest,
} from "@agent-management-platform/types";

function buildBaseUrl(params: GetAgentResourceConfigsPathParams): string {
  const orgName = params.orgName ?? "default";
  const projName = params.projName ?? "default";
  const agentName = params.agentName;
  if (!agentName) {
    throw new Error("agentName is required");
  }
  return `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/resource-configs`;
}

export async function getAgentResourceConfigs(
  params: GetAgentResourceConfigsPathParams,
  query?: GetAgentResourceConfigsQuery,
  getToken?: () => Promise<string>
): Promise<AgentResourceConfigsResponse> {
  const baseUrl = buildBaseUrl(params);
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.environment !== undefined) {
    searchParams.environment = query.environment;
  }

  const res = await httpGET(baseUrl, {
    token,
    searchParams: Object.keys(searchParams).length > 0 ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateAgentResourceConfigs(
  params: UpdateAgentResourceConfigsPathParams,
  body: UpdateAgentResourceConfigsRequest,
  query?: UpdateAgentResourceConfigsQuery,
  getToken?: () => Promise<string>
): Promise<AgentResourceConfigsResponse> {
  const baseUrl = buildBaseUrl(params);
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.environment !== undefined) {
    searchParams.environment = query.environment;
  }

  const res = await httpPUT(baseUrl, cloneDeep(body), {
    token,
    searchParams: Object.keys(searchParams).length > 0 ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

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
import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
import type {
  AgentModelConfigListResponse,
  AgentModelConfigResponse,
  CreateAgentModelConfigPathParams,
  CreateAgentModelConfigRequest,
  DeleteAgentModelConfigPathParams,
  GetAgentModelConfigPathParams,
  ListAgentModelConfigsPathParams,
  ListAgentModelConfigsQuery,
  UpdateAgentModelConfigPathParams,
  UpdateAgentModelConfigRequest,
} from "@agent-management-platform/types";

function encodeRequired(value: string | undefined, label: string): string {
  if (!value) {
    throw new Error(`Missing required parameter: ${label}`);
  }
  return encodeURIComponent(value);
}

function buildBaseUrl(params: {
  orgName?: string;
  projName?: string;
  agentName?: string;
}): string {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  return `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/model-configs`;
}

export async function listAgentModelConfigs(
  params: ListAgentModelConfigsPathParams,
  query?: ListAgentModelConfigsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentModelConfigListResponse> {
  const baseUrl = buildBaseUrl(params);
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(baseUrl, { token, searchParams });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createAgentModelConfig(
  params: CreateAgentModelConfigPathParams,
  body: CreateAgentModelConfigRequest,
  getToken?: () => Promise<string>,
): Promise<AgentModelConfigResponse> {
  const baseUrl = buildBaseUrl(params);
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(baseUrl, cloneDeep(body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentModelConfig(
  params: GetAgentModelConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentModelConfigResponse> {
  const configId = encodeRequired(params.configId, "configId");
  const baseUrl = `${buildBaseUrl(params)}/${configId}`;
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(baseUrl, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateAgentModelConfig(
  params: UpdateAgentModelConfigPathParams,
  body: UpdateAgentModelConfigRequest,
  getToken?: () => Promise<string>,
): Promise<AgentModelConfigResponse> {
  const configId = encodeRequired(params.configId, "configId");
  const baseUrl = `${buildBaseUrl(params)}/${configId}`;
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(baseUrl, cloneDeep(body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteAgentModelConfig(
  params: DeleteAgentModelConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const configId = encodeRequired(params.configId, "configId");
  const baseUrl = `${buildBaseUrl(params)}/${configId}`;
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(baseUrl, { token });
  if (!res.ok) throw await res.json();
}

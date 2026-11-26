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

import type {
  AgentConfigDocument,
  AgentConfigDocumentList,
  CreateLLMConfigAPIKeyPathParams,
  CreateLLMConfigAPIKeyRequest,
  CreateLLMConfigAPIKeyResponse,
  DeleteAgentModelConfigPathParams,
  GetAgentModelConfigPathParams,
  ListAgentModelConfigsPathParams,
  ListAgentModelConfigsQuery,
  ListAPIKeysResponse,
  ListLLMConfigAPIKeysPathParams,
  RevokeLLMConfigAPIKeyPathParams,
  RotateLLMConfigAPIKeyPathParams,
  RotateLLMConfigAPIKeyRequest,
  RotateLLMConfigAPIKeyResponse,
  UpdateAgentModelConfigPathParams,
} from "@agent-management-platform/types";
import {
  agentGatewayPath,
  agentProjectQuery,
  agentRequestOptions,
  encodeRequired,
  httpDELETE,
  httpGET,
  httpPOST,
  httpPUT,
} from "../../shared/utils";

function configsPath(params: { agentName?: string }, suffix = ""): string {
  return agentGatewayPath(params, `/model-configs${suffix}`);
}

function configPath(params: { agentName?: string; configId?: string }, suffix = ""): string {
  return configsPath(params, `/${encodeRequired(params.configId, "configId")}${suffix}`);
}

export async function listAgentModelConfigs(
  params: ListAgentModelConfigsPathParams,
  query?: ListAgentModelConfigsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentConfigDocumentList> {
  const token = getToken ? await getToken() : undefined;
  const searchParams = agentProjectQuery(params, {
    limit: query?.limit !== undefined ? String(query.limit) : undefined,
    offset: query?.offset !== undefined ? String(query.offset) : undefined,
  });
  const res = await httpGET(configsPath(params), { token, searchParams });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createAgentModelConfig(
  params: ListAgentModelConfigsPathParams,
  body: AgentConfigDocument,
  getToken?: () => Promise<string>,
): Promise<AgentConfigDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(configsPath(params), body, agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentModelConfig(
  params: GetAgentModelConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentConfigDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(configPath(params), agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateAgentModelConfig(
  params: UpdateAgentModelConfigPathParams,
  body: AgentConfigDocument,
  getToken?: () => Promise<string>,
): Promise<AgentConfigDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(configPath(params), body, agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteAgentModelConfig(
  params: DeleteAgentModelConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(configPath(params), agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
}

// Per-config API keys (external agents), in the default environment.

export async function listLLMConfigAPIKeys(
  params: ListLLMConfigAPIKeysPathParams,
  getToken?: () => Promise<string>,
): Promise<ListAPIKeysResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(configPath(params, "/api-keys"), agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMConfigAPIKey(
  params: CreateLLMConfigAPIKeyPathParams,
  body: CreateLLMConfigAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateLLMConfigAPIKeyResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(configPath(params, "/api-keys"), body, agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rotateLLMConfigAPIKey(
  params: RotateLLMConfigAPIKeyPathParams,
  body: RotateLLMConfigAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<RotateLLMConfigAPIKeyResponse> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(configPath(params, `/api-keys/${keyName}`), body, agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeLLMConfigAPIKey(
  params: RevokeLLMConfigAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(configPath(params, `/api-keys/${keyName}`), agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
}

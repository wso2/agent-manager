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
  CreateMCPConfigAPIKeyPathParams,
  CreateMCPConfigAPIKeyRequest,
  CreateMCPConfigAPIKeyResponse,
  DeleteAgentMCPConfigPathParams,
  GetAgentMCPConfigPathParams,
  ListAgentMCPConfigsPathParams,
  ListAgentMCPConfigsQuery,
  ListAPIKeysResponse,
  ListMCPConfigAPIKeysPathParams,
  RevokeMCPConfigAPIKeyPathParams,
  RotateMCPConfigAPIKeyPathParams,
  RotateMCPConfigAPIKeyRequest,
  RotateMCPConfigAPIKeyResponse,
  UpdateAgentMCPConfigPathParams,
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
  return agentGatewayPath(params, `/mcp-configs${suffix}`);
}

function configPath(params: { agentName?: string; configId?: string }, suffix = ""): string {
  return configsPath(params, `/${encodeRequired(params.configId, "configId")}${suffix}`);
}

export async function listAgentMCPConfigs(
  params: ListAgentMCPConfigsPathParams,
  query?: ListAgentMCPConfigsQuery,
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

export async function createAgentMCPConfig(
  params: ListAgentMCPConfigsPathParams,
  body: AgentConfigDocument,
  getToken?: () => Promise<string>,
): Promise<AgentConfigDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(configsPath(params), body, agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentMCPConfig(
  params: GetAgentMCPConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentConfigDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(configPath(params), agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateAgentMCPConfig(
  params: UpdateAgentMCPConfigPathParams,
  body: AgentConfigDocument,
  getToken?: () => Promise<string>,
): Promise<AgentConfigDocument> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(configPath(params), body, agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteAgentMCPConfig(
  params: DeleteAgentMCPConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(configPath(params), agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
}

// Per-config API keys (external agents), in the default environment.

export async function listMCPConfigAPIKeys(
  params: ListMCPConfigAPIKeysPathParams,
  getToken?: () => Promise<string>,
): Promise<ListAPIKeysResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(configPath(params, "/api-keys"), agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createMCPConfigAPIKey(
  params: CreateMCPConfigAPIKeyPathParams,
  body: CreateMCPConfigAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateMCPConfigAPIKeyResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(configPath(params, "/api-keys"), body, agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rotateMCPConfigAPIKey(
  params: RotateMCPConfigAPIKeyPathParams,
  body: RotateMCPConfigAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<RotateMCPConfigAPIKeyResponse> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(configPath(params, `/api-keys/${keyName}`), body, agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeMCPConfigAPIKey(
  params: RevokeMCPConfigAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(configPath(params, `/api-keys/${keyName}`), agentRequestOptions(params, token));
  if (!res.ok) throw await res.json();
}

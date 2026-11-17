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
import {
  encodeRequired,
  httpDELETE,
  httpGET,
  httpPOST,
  httpPUT,
  SERVICE_BASE,
} from "../../shared/utils";
import { normalizeAgentModelConfigResponse } from "./agent-model-configs";
import type {
  AgentMCPConfigListResponse,
  AgentMCPConfigResponse,
  CreateAgentMCPConfigPathParams,
  CreateAgentMCPConfigRequest,
  CreateMCPConfigAPIKeyPathParams,
  CreateMCPConfigAPIKeyRequest,
  CreateMCPConfigAPIKeyResponse,
  DeleteAgentMCPConfigPathParams,
  EnvMCPProxyConfigRequest,
  EnvProviderConfigMappings,
  GetAgentMCPConfigPathParams,
  ListAgentMCPConfigsPathParams,
  ListAgentMCPConfigsQuery,
  ListAPIKeysResponse,
  ListMCPConfigAPIKeysPathParams,
  ProviderConfig,
  RevokeMCPConfigAPIKeyPathParams,
  RotateMCPConfigAPIKeyPathParams,
  RotateMCPConfigAPIKeyRequest,
  RotateMCPConfigAPIKeyResponse,
  UpdateAgentMCPConfigPathParams,
  UpdateAgentMCPConfigRequest,
} from "@agent-management-platform/types";

function buildBaseUrl(params: {
  orgName?: string;
  projName?: string;
  agentName?: string;
}): string {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  return `${SERVICE_BASE}/orgs/${org}/projects/${proj}/agents/${agent}/mcp-configs`;
}

function getMCPProxyName(mapping: EnvMCPProxyConfigRequest): string | undefined {
  return (
    mapping.proxyName ??
    mapping.proxyId ??
    mapping.mcpProxyName ??
    mapping.mcpProxyId ??
    mapping.providerName
  );
}

function toModelConfigRequest<T extends CreateAgentMCPConfigRequest | UpdateAgentMCPConfigRequest>(
  body: T,
): T {
  const next = cloneDeep(body);
  if (!next.envMappings) {
    return next;
  }

  next.envMappings = Object.fromEntries(
    Object.entries(next.envMappings).map(([envName, mapping]) => [
      envName,
      {
        ...mapping,
        providerName: getMCPProxyName(mapping),
      },
    ]),
  ) as T["envMappings"];
  return next;
}

function normalizeAgentMCPConfigResponse(raw: AgentMCPConfigResponse): AgentMCPConfigResponse {
  const response = normalizeAgentModelConfigResponse(raw) as AgentMCPConfigResponse;
  const envMappings = response.envMappings ?? {};

  return {
    ...response,
    envMappings: Object.fromEntries(
      Object.entries(envMappings).map(([envName, mapping]) => {
        const config = mapping.configuration;
        const proxyName =
          config?.proxyName ??
          config?.proxyId ??
          config?.mcpProxyName ??
          config?.mcpProxyId ??
          config?.providerName;
        return [
          envName,
          {
            ...mapping,
            configuration: config
              ? ({
                  ...config,
                  proxyName,
                  proxyId: proxyName,
                  mcpProxyName: proxyName,
                  mcpProxyId: proxyName,
                } as ProviderConfig)
              : undefined,
          },
        ];
      }),
    ) as Record<string, EnvProviderConfigMappings>,
  };
}

export async function listAgentMCPConfigs(
  params: ListAgentMCPConfigsPathParams,
  query?: ListAgentMCPConfigsQuery,
  getToken?: () => Promise<string>,
): Promise<AgentMCPConfigListResponse> {
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
  const list = (await res.json()) as AgentMCPConfigListResponse;
  return {
    ...list,
    configs: list.configs.map((config) => ({
      ...config,
      type: "mcp",
    })),
  };
}

export async function createAgentMCPConfig(
  params: CreateAgentMCPConfigPathParams,
  body: CreateAgentMCPConfigRequest,
  getToken?: () => Promise<string>,
): Promise<AgentMCPConfigResponse> {
  const baseUrl = buildBaseUrl(params);
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(baseUrl, toModelConfigRequest({ ...body, type: "mcp" }), {
    token,
  });
  if (!res.ok) throw await res.json();
  return normalizeAgentMCPConfigResponse(await res.json());
}

export async function getAgentMCPConfig(
  params: GetAgentMCPConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentMCPConfigResponse> {
  const configId = encodeRequired(params.configId, "configId");
  const baseUrl = `${buildBaseUrl(params)}/${configId}`;
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(baseUrl, { token });
  if (!res.ok) throw await res.json();
  return normalizeAgentMCPConfigResponse(await res.json());
}

export async function updateAgentMCPConfig(
  params: UpdateAgentMCPConfigPathParams,
  body: UpdateAgentMCPConfigRequest,
  getToken?: () => Promise<string>,
): Promise<AgentMCPConfigResponse> {
  const configId = encodeRequired(params.configId, "configId");
  const baseUrl = `${buildBaseUrl(params)}/${configId}`;
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(baseUrl, toModelConfigRequest(body), { token });
  if (!res.ok) throw await res.json();
  return normalizeAgentMCPConfigResponse(await res.json());
}

export async function deleteAgentMCPConfig(
  params: DeleteAgentMCPConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const configId = encodeRequired(params.configId, "configId");
  const baseUrl = `${buildBaseUrl(params)}/${configId}`;
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(baseUrl, { token });
  if (!res.ok) throw await res.json();
}

// -----------------------------------------------------------------------------
// Per-config MCP API keys (external agents)
// -----------------------------------------------------------------------------

function mcpConfigAPIKeysUrl(params: {
  orgName?: string;
  projName?: string;
  agentName?: string;
  configId?: string;
  envName?: string;
}): string {
  const configId = encodeRequired(params.configId, "configId");
  const envName = encodeRequired(params.envName, "envName");
  return `${buildBaseUrl(params)}/${configId}/environments/${envName}/api-keys`;
}

export async function listMCPConfigAPIKeys(
  params: ListMCPConfigAPIKeysPathParams,
  getToken?: () => Promise<string>,
): Promise<ListAPIKeysResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(mcpConfigAPIKeysUrl(params), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createMCPConfigAPIKey(
  params: CreateMCPConfigAPIKeyPathParams,
  body: CreateMCPConfigAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateMCPConfigAPIKeyResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(mcpConfigAPIKeysUrl(params), body, { token });
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
  const res = await httpPUT(
    `${mcpConfigAPIKeysUrl(params)}/${keyName}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeMCPConfigAPIKey(
  params: RevokeMCPConfigAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${mcpConfigAPIKeysUrl(params)}/${keyName}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

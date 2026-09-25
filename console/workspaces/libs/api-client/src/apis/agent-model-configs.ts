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
} from "../utils";
import type {
  AgentModelConfigListResponse,
  AgentModelConfigResponse,
  CreateAgentModelConfigPathParams,
  CreateAgentModelConfigRequest,
  CreateLLMConfigAPIKeyPathParams,
  CreateLLMConfigAPIKeyRequest,
  CreateLLMConfigAPIKeyResponse,
  DeleteAgentModelConfigPathParams,
  EnvProviderConfigMappings,
  ListAPIKeysResponse,
  ListLLMConfigAPIKeysPathParams,
  ProviderConfig,
  GetAgentModelConfigPathParams,
  ListAgentModelConfigsPathParams,
  ListAgentModelConfigsQuery,
  RevokeLLMConfigAPIKeyPathParams,
  RotateLLMConfigAPIKeyPathParams,
  RotateLLMConfigAPIKeyRequest,
  RotateLLMConfigAPIKeyResponse,
  UpdateAgentModelConfigPathParams,
  UpdateAgentModelConfigRequest,
} from "@agent-management-platform/types";

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
  return normalizeAgentModelConfigResponse(await res.json());
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
  return normalizeAgentModelConfigResponse(await res.json());
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
  return normalizeAgentModelConfigResponse(await res.json());
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

// -----------------------------------------------------------------------------
// Per-config LLM API keys (external agents)
// -----------------------------------------------------------------------------

function llmConfigAPIKeysUrl(params: {
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

export async function listLLMConfigAPIKeys(
  params: ListLLMConfigAPIKeysPathParams,
  getToken?: () => Promise<string>,
): Promise<ListAPIKeysResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(llmConfigAPIKeysUrl(params), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMConfigAPIKey(
  params: CreateLLMConfigAPIKeyPathParams,
  body: CreateLLMConfigAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateLLMConfigAPIKeyResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(llmConfigAPIKeysUrl(params), body, { token });
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
  const res = await httpPUT(`${llmConfigAPIKeysUrl(params)}/${keyName}`, body, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeLLMConfigAPIKey(
  params: RevokeLLMConfigAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(`${llmConfigAPIKeysUrl(params)}/${keyName}`, {
    token,
  });
  if (!res.ok) throw await res.json();
}

export function normalizeAgentModelConfigResponse(
  raw: AgentModelConfigResponse & {
    envModelConfig?: Record<string, {
      environmentName: string;
      llmProxy?: {
        proxyUrl?: string;
        proxyUuid?: string;
        proxyName?: string;
        providerName?: string;
        policies?: unknown[];
        apiKey?: string;
        authHeaderName?: string;
        authIn?: string;
      };
      configuration?: unknown;
    }>;
  },
): AgentModelConfigResponse {
  if (raw.envMappings) {
    return raw;
  }

  const envModelConfig = raw.envModelConfig ?? {};
  return {
    ...raw,
    envMappings: Object.fromEntries(
      Object.entries(envModelConfig).map(([envName, mapping]) => [
        envName,
        {
          environmentName: mapping.environmentName ?? envName,
          configuration: (mapping.configuration ?? (mapping.llmProxy
            ? {
                providerName: mapping.llmProxy.providerName,
                proxyUuid: mapping.llmProxy.proxyUuid,
                proxyName: mapping.llmProxy.proxyName,
                url: mapping.llmProxy.proxyUrl,
                // Keyed off the header rather than the key: the server reports
                // the name on every read but the value only at creation.
                authInfo: mapping.llmProxy.authHeaderName
                  ? {
                      type: "apikey",
                      in: mapping.llmProxy.authIn || "header",
                      name: mapping.llmProxy.authHeaderName,
                      value: mapping.llmProxy.apiKey,
                    }
                  : undefined,
                policies: mapping.llmProxy.policies,
              }
            : undefined)) as ProviderConfig | undefined,
        },
      ]),
    ) as Record<string, EnvProviderConfigMappings>,
  };
}

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

import type {
  AddAgentMCPProxyPathParams,
  AddAgentMCPProxyRequest,
  AgentMCPProxyListResponse,
  AgentMCPConfigResponse,
  CreateAgentMCPConfigRequest,
  ListAgentMCPProxiesPathParams,
} from "@agent-management-platform/types";
import { listEnvironments } from "./deployments";
import {
  createAgentMCPConfig,
  getAgentMCPConfig,
  listAgentMCPConfigs,
} from "./agent-mcp-configs";

export async function listAgentMCPProxies(
  params: ListAgentMCPProxiesPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentMCPProxyListResponse> {
  const list = await listAgentMCPConfigs(
    params,
    { limit: 1000, offset: 0 },
    getToken,
  );
  const details = await Promise.all(
    list.configs.map((config) =>
      getAgentMCPConfig({ ...params, configId: config.uuid }, getToken),
    ),
  );
  return {
    count: details.length,
    list: details.map(toAgentMCPProxyListItem),
  };
}

export async function addAgentMCPProxy(
  params: AddAgentMCPProxyPathParams,
  body: AddAgentMCPProxyRequest,
  getToken?: () => Promise<string>,
): Promise<AgentMCPConfigResponse> {
  const envs = await listEnvironments({ orgName: params.orgName }, getToken);
  const envMappings: CreateAgentMCPConfigRequest["envMappings"] = {};
  for (const env of envs) {
    envMappings[env.name] = {
      proxyId: body.proxyId,
      configuration: {},
    };
  }

  return createAgentMCPConfig(
    params,
    {
      name: body.proxyId,
      type: "mcp",
      envMappings,
      environmentVariables: body.environmentVariables,
    },
    getToken,
  );
}

function toAgentMCPProxyListItem(config: AgentMCPConfigResponse) {
  const firstMapping = Object.values(config.envMappings ?? {})[0];
  const firstConfig = firstMapping?.configuration;
  let context: string | undefined;
  if (firstConfig?.url) {
    try {
      context = new URL(firstConfig.url).pathname;
    } catch {
      context = firstConfig.url;
    }
  }
  return {
    id: config.uuid,
    name: config.name,
    description: config.description,
    proxyId: getMCPProxyName(firstConfig),
    proxyName: getMCPProxyName(firstConfig),
    proxyUrl: firstConfig?.url,
    context,
    status: firstConfig ? "active" : undefined,
    envMappings: config.envMappings,
    environmentVariables: config.environmentVariables,
    createdAt: config.createdAt,
    version: undefined,
  };
}

function getMCPProxyName(config: AgentMCPConfigResponse["envMappings"][string]["configuration"]): string | undefined {
  return (
    config?.proxyName ??
    config?.proxyId ??
    config?.mcpProxyName ??
    config?.mcpProxyId ??
    config?.providerName
  );
}

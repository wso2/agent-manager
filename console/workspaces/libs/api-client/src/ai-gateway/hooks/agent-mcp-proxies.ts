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

import { useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import type {
  AddAgentMCPProxyPathParams,
  AddAgentMCPProxyRequest,
  AgentMCPConfigResponse,
  AgentMCPProxyListResponse,
  ListAgentMCPProxiesPathParams,
} from "@agent-management-platform/types";
import { addAgentMCPProxy, listAgentMCPProxies } from "../apis/agent-mcp-proxies";
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";

export function useListAgentMCPProxies(params: ListAgentMCPProxiesPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentMCPProxyListResponse>({
    queryKey: ["agent-mcp-proxies", params],
    queryFn: () => listAgentMCPProxies(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
  });
}

export function useAddAgentMCPProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentMCPConfigResponse,
    unknown,
    { params: AddAgentMCPProxyPathParams; body: AddAgentMCPProxyRequest }
  >({
    action: { verb: "create", target: "MCP server" },
    mutationFn: ({ params, body }) => addAgentMCPProxy(params, body, getToken),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["agent-mcp-proxies", variables.params],
      });
      queryClient.invalidateQueries({ queryKey: ["agent-mcp-configs"] });
    },
  });
}

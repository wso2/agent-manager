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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { type ReactNode } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useDeleteMCPProxy, useListMCPProxies } from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { MCPProxyListTable } from "./MCPProxyListTable";

interface MCPProxyTableProps {
  /** CTA shown in the empty state; the page owns how a server gets registered. */
  createAction?: ReactNode;
}

export function MCPProxyTable({ createAction }: MCPProxyTableProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const { data, isLoading, error } = useListMCPProxies({ orgName: orgId });
  const { mutate: deleteMCPProxy } = useDeleteMCPProxy();

  return (
    <MCPProxyListTable
      proxies={data?.list ?? []}
      isLoading={isLoading}
      error={error}
      emptyState={{
        title: "No MCP Servers Yet",
        description: "Add an MCP Server to provide tools for agents.",
        action: createAction,
      }}
      onOpen={(proxy) =>
        navigate(
          generatePath(absoluteRouteMap.children.org.children.mcpProxies.children.view.path, {
            orgId,
            proxyId: proxy.id,
          }),
        )
      }
      canDelete={() => true}
      deleteDescription={(_, displayName) =>
        `Are you sure you want to delete ${displayName}? This action cannot be undone.`
      }
      onDelete={(proxy) => proxy.id && deleteMCPProxy({ orgName: orgId, proxyId: proxy.id })}
    />
  );
}


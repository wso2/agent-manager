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

import { useMemo, type ReactNode } from "react";
import { Box, Divider, Stack, Tab, Tabs } from "@wso2/oxygen-ui";
import {
  generatePath,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import {
  useDeleteProjectMCPProxy,
  useListAllMCPProxies,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type MCPProxyListItem,
} from "@agent-management-platform/types";
import { MCPProxyListTable } from "./MCPProxyListTable";

/** Which servers the list shows: all of them, the project's own, or the organization's. */
type ScopeFilter = "all" | "project" | "organization";

const SCOPE_FILTERS: { value: ScopeFilter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "project", label: "Project" },
  { value: "organization", label: "From organization" },
];

function inScope(proxy: MCPProxyListItem, scope: ScopeFilter): boolean {
  if (scope === "project") return !!proxy.projectName;
  if (scope === "organization") return !proxy.projectName;
  return true;
}

/**
 * The MCP servers a project can use: the org's own and the project's. A project
 * server opens its project page; an org server opens the org page.
 */
export function ProjectMCPProxyTable({
  createAction,
}: {
  createAction?: ReactNode;
}) {
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();
  // Kept in the URL, as the other tabbed pages do, so it survives a reload.
  const [searchParams, setSearchParams] = useSearchParams();
  const scopeParam = searchParams.get("scope");
  const scope: ScopeFilter = SCOPE_FILTERS.some(
    (item) => item.value === scopeParam,
  )
    ? (scopeParam as ScopeFilter)
    : "all";
  const setScope = (next: ScopeFilter) =>
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev);
        if (next === "all") params.delete("scope");
        else params.set("scope", next);
        return params;
      },
      { replace: true },
    );
  // Every page: the org's servers and the project's share the list, and search, the scope
  // tabs and their counts are worked out on what is loaded.
  const { data, isLoading, error } = useListAllMCPProxies(
    { orgName: orgId },
    { project: projectId },
  );
  const { mutate: deleteProjectProxy } = useDeleteProjectMCPProxy();

  const proxies = useMemo(() => data?.list ?? [], [data]);
  const scopeCounts = useMemo(
    () =>
      Object.fromEntries(
        SCOPE_FILTERS.map((item) => [
          item.value,
          proxies.filter((proxy) => inScope(proxy, item.value)).length,
        ]),
      ) as Record<ScopeFilter, number>,
    [proxies],
  );
  const scoped = useMemo(
    () => proxies.filter((proxy) => inScope(proxy, scope)),
    [proxies, scope],
  );

  const openProxy = (proxy: MCPProxyListItem) =>
    navigate(
      proxy.projectName
        ? generatePath(
            absoluteRouteMap.children.org.children.projects.children.mcpProxies
              .children.view.path,
            { orgId, projectId, proxyId: proxy.id },
          )
        : generatePath(
            absoluteRouteMap.children.org.children.mcpProxies.children.view
              .path,
            { orgId, proxyId: proxy.id },
          ),
    );

  return (
    <Stack spacing={2}>
      {proxies.length > 0 && (
        <Box>
          <Tabs
            value={scope}
            onChange={(_, value: ScopeFilter) => setScope(value)}
          >
            {SCOPE_FILTERS.map((item) => (
              <Tab
                key={item.value}
                value={item.value}
                label={`${item.label} (${scopeCounts[item.value]})`}
              />
            ))}
          </Tabs>
          <Divider />
        </Box>
      )}
      <MCPProxyListTable
        proxies={scoped}
        isEmpty={!proxies.length}
        isLoading={isLoading}
        error={error}
        emptyState={{
          title: "No MCP Servers Yet",
          description:
            "MCP servers registered for the organization or this project, and those published from its components, show up here.",
          action: createAction,
        }}
        filterEmptyState={
          scope === "project"
            ? {
                title: "No project MCP servers yet",
                description:
                  "Register an MCP server for this project, or publish one from a component.",
              }
            : {
                title: "No organization MCP servers",
                description:
                  "MCP servers registered for the organization show up here.",
              }
        }
        onOpen={openProxy}
        caption={(proxy) => (proxy.projectName ? "Project" : "Organization")}
        // Only the project's own servers are deleted here; an org server is every project's,
        // so it is deleted from the org page.
        canDelete={(proxy) => !!proxy.projectName}
        deleteDescription={(_, displayName) =>
          `Are you sure you want to delete ${displayName}? It is removed from every environment it is deployed to. This action cannot be undone.`
        }
        onDelete={(proxy) =>
          proxy.id &&
          deleteProjectProxy({
            orgName: orgId,
            projName: projectId,
            proxyId: proxy.id,
          })
        }
      />
    </Stack>
  );
}

export default ProjectMCPProxyTable;

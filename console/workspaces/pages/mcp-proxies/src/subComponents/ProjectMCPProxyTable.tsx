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

import { useMemo, useState, type ReactNode } from "react";
import {
  Avatar,
  Box,
  Divider,
  IconButton,
  ListingTable,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash } from "@wso2/oxygen-ui-icons-react";
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
  formatRelativeTime,
  getAvatarInitials,
  ResourceListShell,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type MCPProxyListItem,
} from "@agent-management-platform/types";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { MCPLogo } from "../components/MCPLogo";

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
  const [searchValue, setSearchValue] = useState("");
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
  const { addConfirmation } = useConfirmationDialog();
  const { canWrite } = useResourceAccess("mcp_server");

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

  const filteredProxies = useMemo(() => {
    const term = searchValue.trim().toLowerCase();
    return proxies.filter(
      (proxy: MCPProxyListItem) =>
        inScope(proxy, scope) &&
        (!term ||
          [
            proxy.name,
            proxy.id,
            proxy.version,
            proxy.context,
            proxy.description,
          ]
            .filter(Boolean)
            .join(" ")
            .toLowerCase()
            .includes(term)),
    );
  }, [proxies, scope, searchValue]);

  const openProxy = (proxy: MCPProxyListItem) => {
    if (!proxy.id) return;
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
  };

  // Only the project's own servers are deleted here; an org server is every project's, so it
  // is deleted from the org page.
  const confirmDelete = (proxy: MCPProxyListItem, displayName: string) => {
    const proxyId = proxy.id;
    if (!proxyId || !proxy.projectName) return;
    addConfirmation({
      analytics: { entity: "mcp-proxy", action: "delete" },
      title: "Delete MCP Server",
      description: `Are you sure you want to delete ${displayName}? It is removed from every environment it is deployed to. This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () =>
        deleteProjectProxy({ orgName: orgId, projName: projectId, proxyId }),
    });
  };

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
      <ResourceListShell
        resourceName="mcp-proxies"
        searchValue={searchValue}
        onSearchChange={setSearchValue}
        searchPlaceholder="Search MCP servers"
        error={error}
        isLoading={isLoading}
        isEmpty={!proxies.length}
        isSearchEmpty={!filteredProxies.length}
        emptyState={{
          illustration: <MCPLogo size={64} />,
          title: "No MCP Servers Yet",
          description:
            "MCP servers registered for the organization or this project, and those published from its components, show up here.",
          action: createAction,
        }}
        searchEmptyState={
          searchValue.trim()
            ? {
                illustration: <Plus size={64} />,
                title: "No MCP servers match your search",
                description:
                  "Try a different keyword or clear the search filter.",
              }
            : {
                illustration: <MCPLogo size={64} />,
                title:
                  scope === "project"
                    ? "No project MCP servers yet"
                    : "No organization MCP servers",
                description:
                  scope === "project"
                    ? "Register an MCP server for this project, or publish one from a component."
                    : "MCP servers registered for the organization show up here.",
              }
        }
      >
        <ListingTable variant="card">
          <ListingTable.Head>
            <ListingTable.Row>
              <ListingTable.Cell>Name</ListingTable.Cell>
              <ListingTable.Cell width="160px">Version</ListingTable.Cell>
              <ListingTable.Cell width="140px" align="right">
                Last Updated
              </ListingTable.Cell>
              <ListingTable.Cell width="96px" align="right">
                Actions
              </ListingTable.Cell>
            </ListingTable.Row>
          </ListingTable.Head>
          <ListingTable.Body>
            {filteredProxies.map((proxy: MCPProxyListItem) => {
              const displayName = proxy.name ?? proxy.id ?? "MCP Server";
              return (
                <ListingTable.Row
                  key={`${proxy.projectName ?? ""}:${proxy.id ?? displayName}`}
                  variant="card"
                  hover
                  clickable={!!proxy.id}
                  onClick={() => openProxy(proxy)}
                >
                  <ListingTable.Cell>
                    <Stack direction="row" alignItems="center" spacing={2}>
                      <Avatar
                        sx={{
                          bgcolor: "primary.main",
                          color: "primary.contrastText",
                          fontSize: 16,
                          height: 36,
                          width: 36,
                          flexShrink: 0,
                        }}
                      >
                        {getAvatarInitials(displayName, { fallback: "MS" })}
                      </Avatar>
                      <Stack spacing={0.25} minWidth={0}>
                        <Typography variant="body2" fontWeight={500} noWrap>
                          {displayName}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {proxy.projectName ? "Project" : "Organization"}
                        </Typography>
                      </Stack>
                    </Stack>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="body2">
                      {proxy.version ?? "-"}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    <Typography variant="caption" color="text.secondary">
                      {formatRelativeTime(proxy.updatedAt)}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell
                    align="right"
                    onClick={(event) => event.stopPropagation()}
                  >
                    {proxy.id && proxy.projectName && canWrite ? (
                      <Tooltip title="Delete MCP Server">
                        <IconButton
                          color="error"
                          size="small"
                          onClick={() => confirmDelete(proxy, displayName)}
                        >
                          <Trash size={16} />
                        </IconButton>
                      </Tooltip>
                    ) : null}
                  </ListingTable.Cell>
                </ListingTable.Row>
              );
            })}
          </ListingTable.Body>
        </ListingTable>
      </ResourceListShell>
    </Stack>
  );
}

export default ProjectMCPProxyTable;

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
  Chip,
  IconButton,
  ListingTable,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteMCPProxy,
  useDeleteProjectMCPProxy,
  useListEnvironments,
  useListMCPProxies,
} from "@agent-management-platform/api-client";
import {
  environmentLabel,
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
import { proxySourceLabel } from "./projectBinding";

/** The most proxies one list request returns (the service's MaxLimit). */
const LIST_LIMIT = 100;

/**
 * The MCP servers a project can use: the org's own and the project's. A project
 * server opens its project page; an org server opens the org page.
 */
export function ProjectMCPProxyTable({ createAction }: { createAction?: ReactNode }) {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const navigate = useNavigate();
  const [searchValue, setSearchValue] = useState("");
  const { data, isLoading, error } = useListMCPProxies(
    { orgName: orgId },
    { project: projectId, limit: LIST_LIMIT },
  );
  const { data: environments = [] } = useListEnvironments({ orgName: orgId });
  const { mutate: deleteOrgProxy } = useDeleteMCPProxy();
  const { mutate: deleteProjectProxy } = useDeleteProjectMCPProxy();
  const { addConfirmation } = useConfirmationDialog();
  const { canWrite } = useResourceAccess("mcp_server");

  const proxies = useMemo(() => data?.list ?? [], [data]);

  // An environment's display name by its UUID; a project binding also carries its name.
  const envLabel = useMemo(() => {
    const byId = new Map(environments.map((env) => [env.id, env]));
    return (uuid: string, name?: string) =>
      environmentLabel(byId.get(uuid)) || name || uuid;
  }, [environments]);

  const filteredProxies = useMemo(() => {
    const term = searchValue.trim().toLowerCase();
    if (!term) return proxies;
    return proxies.filter((proxy: MCPProxyListItem) =>
      [
        proxy.name,
        proxy.id,
        proxy.description,
        proxy.context,
        proxySourceLabel(proxy),
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(term),
    );
  }, [proxies, searchValue]);

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

  const confirmDelete = (proxy: MCPProxyListItem, displayName: string) => {
    const proxyId = proxy.id;
    if (!proxyId) return;
    const isProjectProxy = !!proxy.projectName;
    addConfirmation({
      analytics: { entity: "mcp-proxy", action: "delete" },
      title: "Delete MCP Server",
      description: isProjectProxy
        ? `Are you sure you want to delete ${displayName}? It is removed from every environment it is deployed to. This action cannot be undone.`
        : `${displayName} is registered for the whole organization. Deleting it removes it from every project. This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () =>
        isProjectProxy
          ? deleteProjectProxy({ orgName: orgId, projName: projectId, proxyId })
          : deleteOrgProxy({ orgName: orgId, proxyId }),
    });
  };

  return (
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
      searchEmptyState={{
        illustration: <Plus size={64} />,
        title: "No MCP servers match your search",
        description: "Try a different keyword or clear the search filter.",
      }}
    >
      <ListingTable variant="card">
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Name</ListingTable.Cell>
            <ListingTable.Cell width="220px">Source</ListingTable.Cell>
            <ListingTable.Cell width="240px">Environments</ListingTable.Cell>
            <ListingTable.Cell width="140px" align="right">
              Created
            </ListingTable.Cell>
            <ListingTable.Cell width="96px" align="right">
              Actions
            </ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {filteredProxies.map((proxy: MCPProxyListItem) => {
            const displayName = proxy.name ?? proxy.id ?? "MCP Server";
            const boundEnvironments = proxy.environments ?? [];
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
                  <Typography variant="body2" noWrap>
                    {proxySourceLabel(proxy)}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  {boundEnvironments.length ? (
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                      {boundEnvironments.map((env) => (
                        <Chip
                          key={env.environmentUuid}
                          label={envLabel(env.environmentUuid, env.environmentName)}
                          size="small"
                          variant="outlined"
                          color="success"
                        />
                      ))}
                    </Stack>
                  ) : (
                    <Typography variant="body2" color="text.secondary">
                      -
                    </Typography>
                  )}
                </ListingTable.Cell>
                <ListingTable.Cell align="right">
                  <Typography variant="caption" color="text.secondary">
                    {formatRelativeTime(proxy.createdAt)}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell
                  align="right"
                  onClick={(event) => event.stopPropagation()}
                >
                  {proxy.id && canWrite ? (
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
  );
}

export default ProjectMCPProxyTable;

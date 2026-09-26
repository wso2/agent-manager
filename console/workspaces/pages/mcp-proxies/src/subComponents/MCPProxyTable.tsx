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

import { useMemo, useState } from "react";
import {
  Avatar,
  IconButton,
  ListingTable,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useNavigate, useParams } from "react-router-dom";
import { useDeleteMCPProxy, useListMCPProxies } from "@agent-management-platform/api-client";
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
import { MCPLogo } from "../components/MCPLogo";

export function MCPProxyTable() {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const [searchValue, setSearchValue] = useState("");
  const routeParams = { orgId: orgId ?? "" };
  const { data, isLoading, error } = useListMCPProxies({ orgName: orgId });
  const { mutate: deleteMCPProxy } = useDeleteMCPProxy();
  const { addConfirmation } = useConfirmationDialog();

  const proxies = useMemo(() => data?.list ?? [], [data]);
  const filteredProxies = useMemo(() => {
    const term = searchValue.trim().toLowerCase();
    if (!term) return proxies;
    return proxies.filter((proxy: MCPProxyListItem) =>
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
        .includes(term),
    );
  }, [proxies, searchValue]);

  return (
    <ResourceListShell
      resourceName="mcp-proxies"
      searchValue={searchValue}
      onSearchChange={setSearchValue}
      searchPlaceholder="Search MCP servers..."
      addButton={{
        label: "Register MCP Server",
        component: Link,
        to: generatePath(
          absoluteRouteMap.children.org.children.mcpProxies.children.add.path,
          routeParams,
        ),
      }}
      error={error}
      isLoading={isLoading}
      isEmpty={!proxies.length}
      isSearchEmpty={!filteredProxies.length}
      emptyState={{
        illustration: <MCPLogo size={64} />,
        title: "No MCP Servers Yet",
        description: "Add an MCP Server to provide tools for agents.",
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
            const proxyId = proxy.id;
            return (
              <ListingTable.Row
                key={proxyId ?? displayName}
                variant="card"
                hover
                clickable={!!proxyId}
                onClick={() => {
                  if (!proxyId) return;
                  navigate(
                    generatePath(
                      absoluteRouteMap.children.org.children.mcpProxies
                        .children.view.path,
                      { orgId, proxyId },
                    ),
                  );
                }}
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
                    <Typography variant="body2" fontWeight={500} noWrap>
                      {displayName}
                    </Typography>
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
                  {proxyId ? (
                    <Tooltip title="Delete MCP Server">
                      <IconButton
                        color="error"
                        size="small"
                        onClick={() =>
                          addConfirmation({
                            analytics: { entity: "mcp-proxy", action: "delete" },
                            title: "Delete MCP Server",
                            description: `Are you sure you want to delete ${displayName}? This action cannot be undone.`,
                            confirmButtonText: "Delete",
                            confirmButtonColor: "error",
                            confirmButtonIcon: <Trash size={16} />,
                            onConfirm: () =>
                              deleteMCPProxy({
                                orgName: orgId,
                                proxyId,
                              }),
                          })
                        }
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

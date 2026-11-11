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
  IconButton,
  ListingTable,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import {
  formatRelativeTime,
  getAvatarInitials,
  ResourceListShell,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import { type MCPProxyListItem } from "@agent-management-platform/types";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { MCPLogo } from "../components/MCPLogo";

/** An empty state of the list: what it shows and, optionally, what the user can do. */
interface ListEmptyState {
  title: string;
  description: string;
  action?: ReactNode;
}

/** Whether the proxy matches a lowercased search term, on the fields the list shows or links. */
export function matchesMCPProxySearch(proxy: MCPProxyListItem, term: string): boolean {
  if (!term) return true;
  return [proxy.name, proxy.id, proxy.version, proxy.context, proxy.description]
    .filter(Boolean)
    .join(" ")
    .toLowerCase()
    .includes(term);
}

export interface MCPProxyListTableProps {
  /** The servers to list, already narrowed by any filter the page applies. */
  proxies: MCPProxyListItem[];
  /** Whether there is nothing to list at all; defaults to an empty `proxies`. */
  isEmpty?: boolean;
  isLoading: boolean;
  error: unknown;
  /** Shown when there is nothing to list at all. */
  emptyState: ListEmptyState;
  /** Shown when the page's own filter, not the search, leaves nothing to list. */
  filterEmptyState?: ListEmptyState;
  onOpen: (proxy: MCPProxyListItem) => void;
  /** Whether the server can be deleted from this list, for a user who can write. */
  canDelete: (proxy: MCPProxyListItem) => boolean;
  /** The confirmation text for deleting the server. */
  deleteDescription: (proxy: MCPProxyListItem, displayName: string) => string;
  onDelete: (proxy: MCPProxyListItem) => void;
  /** A line under the server's name, e.g. who owns it. */
  caption?: (proxy: MCPProxyListItem) => ReactNode;
}

/** The MCP server list of the org and project pages: search, the table, and delete. */
export function MCPProxyListTable({
  proxies,
  isEmpty = !proxies.length,
  isLoading,
  error,
  emptyState,
  filterEmptyState,
  onOpen,
  canDelete,
  deleteDescription,
  onDelete,
  caption,
}: MCPProxyListTableProps) {
  const [searchValue, setSearchValue] = useState("");
  const { addConfirmation } = useConfirmationDialog();
  const { canWrite } = useResourceAccess("mcp_server");
  const term = searchValue.trim().toLowerCase();

  const filteredProxies = useMemo(
    () => proxies.filter((proxy) => matchesMCPProxySearch(proxy, term)),
    [proxies, term],
  );

  const confirmDelete = (proxy: MCPProxyListItem, displayName: string) =>
    addConfirmation({
      analytics: { entity: "mcp-proxy", action: "delete" },
      title: "Delete MCP Server",
      description: deleteDescription(proxy, displayName),
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => onDelete(proxy),
    });

  return (
    <ResourceListShell
      resourceName="mcp-proxies"
      searchValue={searchValue}
      onSearchChange={setSearchValue}
      searchPlaceholder="Search MCP servers"
      error={error}
      isLoading={isLoading}
      isEmpty={isEmpty}
      isSearchEmpty={!filteredProxies.length}
      emptyState={{ illustration: <MCPLogo size={64} />, ...emptyState }}
      searchEmptyState={
        term || !filterEmptyState
          ? {
              illustration: <Plus size={64} />,
              title: "No MCP servers match your search",
              description: "Try a different keyword or clear the search filter.",
            }
          : { illustration: <MCPLogo size={64} />, ...filterEmptyState }
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
          {filteredProxies.map((proxy) => {
            const displayName = proxy.name ?? proxy.id ?? "MCP Server";
            const nameCaption = caption?.(proxy);
            return (
              <ListingTable.Row
                // An org server and a project server may share an id.
                key={`${proxy.projectName ?? ""}:${proxy.id ?? displayName}`}
                variant="card"
                hover
                clickable={!!proxy.id}
                onClick={() => proxy.id && onOpen(proxy)}
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
                      {nameCaption && (
                        <Typography variant="caption" color="text.secondary">
                          {nameCaption}
                        </Typography>
                      )}
                    </Stack>
                  </Stack>
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography variant="body2">{proxy.version ?? "-"}</Typography>
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
                  {proxy.id && canWrite && canDelete(proxy) ? (
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

export default MCPProxyListTable;

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

import { useEffect, useMemo, useState } from "react";
import {
  Avatar,
  Button,
  Chip,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit, Plus, Search, Server, Trash } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useListEnvironments } from "@agent-management-platform/api-client";
import {
  getErrorMessage,
  IsolationTierChip,
  ResourceListShell,
} from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type Environment,
} from "@agent-management-platform/types";
import { FadeIn } from "@agent-management-platform/views";

interface EnvironmentTableProps {
  onEditEnvironment?: (environment: Environment) => void;
  onCreateEnvironment?: () => void;
  onDeleteEnvironment?: (environment: Environment) => void;
}

export function EnvironmentTable({
  onEditEnvironment,
  onCreateEnvironment,
  onDeleteEnvironment,
}: EnvironmentTableProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);
  const [hoveredId, setHoveredId] = useState<string | null>(null);

  const envViewPath = (envName: string) =>
    orgId
      ? generatePath(
          absoluteRouteMap.children.org.children.environments.children.view
            .path,
          {
            orgId,
            envName,
          },
        )
      : "#";

  const {
    data: environments,
    isLoading,
    error,
  } = useListEnvironments({ orgName: orgId });

  const filtered = useMemo(() => {
    const items = environments ?? [];
    const q = search.trim().toLowerCase();
    if (!q) return items;
    return items.filter(
      (e) =>
        e.name.toLowerCase().includes(q) ||
        (e.displayName ?? "").toLowerCase().includes(q),
    );
  }, [environments, search]);

  const items = environments ?? [];

  useEffect(() => {
    if (page !== 0 && page * rowsPerPage >= filtered.length) setPage(0);
  }, [filtered.length, page, rowsPerPage]);

  const loadingRows = (
    <Stack spacing={1} mt={1}>
      {Array.from({ length: rowsPerPage }).map((_, i) => (
        <Stack
          key={i}
          direction="row"
          alignItems="center"
          spacing={2}
          sx={{
            px: 2,
            py: 1.5,
            borderRadius: 1,
            border: "1px solid",
            borderColor: "divider",
            bgcolor: "background.paper",
          }}
        >
          <Stack
            direction="row"
            alignItems="center"
            spacing={1.5}
            sx={{ flexShrink: 0 }}
          >
            <Skeleton variant="circular" width={36} height={36} />
            <Skeleton variant="text" width={140} height={20} />
          </Stack>
          <Skeleton
            variant="text"
            width={120}
            height={20}
            sx={{ flexShrink: 0 }}
          />
          <Skeleton
            variant="rounded"
            width={80}
            height={24}
            sx={{ flexShrink: 0 }}
          />
          <Skeleton
            variant="rounded"
            width={100}
            height={24}
            sx={{ flexShrink: 0, ml: "auto" }}
          />
        </Stack>
      ))}
    </Stack>
  );

  const createButton = onCreateEnvironment && (
    <Button
      variant="contained"
      startIcon={<Plus size={16} />}
      onClick={onCreateEnvironment}
    >
      Create Environment
    </Button>
  );

  const paginated = filtered.slice(
    page * rowsPerPage,
    page * rowsPerPage + rowsPerPage,
  );

  return (
    <ResourceListShell
      resourceName="environments"
      searchValue={search}
      onSearchChange={setSearch}
      searchPlaceholder="Search environments..."
      searchDisabled={isLoading}
      addButton={
        onCreateEnvironment && items.length > 0
          ? { label: "Create Environment", onClick: onCreateEnvironment }
          : undefined
      }
      error={
        error
          ? new Error(
              `Failed to load environments. ${getErrorMessage(error) || "Please try again."}`,
            )
          : undefined
      }
      isLoading={isLoading}
      isEmpty={items.length === 0}
      isSearchEmpty={filtered.length === 0}
      loadingPlaceholder={loadingRows}
      emptyState={{
        illustration: <Server size={64} />,
        title: "No environments yet",
        description:
          "Create an environment to define where your agents are deployed.",
        action: createButton,
      }}
      searchEmptyState={{
        illustration: <Search size={64} />,
        title: "No environments match your search",
        description: "Try a different keyword or clear the search filter.",
      }}
    >
      <ListingTable variant="card">
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>Environment</ListingTable.Cell>
            <ListingTable.Cell>Data Plane</ListingTable.Cell>
            <ListingTable.Cell align="center" width="160px">
              Type
            </ListingTable.Cell>
            <ListingTable.Cell align="center" width="160px">
              Isolation Tier
            </ListingTable.Cell>
            <ListingTable.Cell align="right" width="160px">
              Created
            </ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {paginated.map((env) => (
            <ListingTable.Row
              key={env.name}
              variant="card"
              hover
              clickable
              onClick={() => navigate(envViewPath(env.name))}
              onMouseEnter={() => setHoveredId(env.name)}
              onMouseLeave={() => setHoveredId(null)}
              onFocus={() => setHoveredId(env.name)}
              onBlur={() => setHoveredId(null)}
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
                    {(env.displayName ?? env.name)?.charAt(0)?.toUpperCase() ??
                      "E"}
                  </Avatar>
                  <Typography variant="body2" fontWeight={500} noWrap>
                    {env.displayName ?? env.name}
                  </Typography>
                </Stack>
              </ListingTable.Cell>

              <ListingTable.Cell>
                <Typography variant="body2" color="text.secondary">
                  {env.dataplaneRef || "—"}
                </Typography>
              </ListingTable.Cell>

              <ListingTable.Cell align="center">
                <Chip
                  label={env.isProduction ? "Production" : "Non-production"}
                  size="small"
                  variant="outlined"
                  color={env.isProduction ? "primary" : "default"}
                />
              </ListingTable.Cell>

              <ListingTable.Cell align="center">
                <IsolationTierChip tier={env.isolationTier} />
              </ListingTable.Cell>

              <ListingTable.Cell
                align="right"
                sx={{ cursor: "default" }}
                onClick={(e) => e.stopPropagation()}
              >
                <Stack
                  direction="row"
                  justifyContent="flex-end"
                  alignItems="center"
                  spacing={1}
                >
                  {hoveredId === env.name ? (
                    <FadeIn>
                      <Stack direction="row" spacing={0.5}>
                        <Tooltip title="Edit environment">
                          <IconButton
                            size="small"
                            onClick={() => onEditEnvironment?.(env)}
                          >
                            <Edit size={16} />
                          </IconButton>
                        </Tooltip>
                        {env.name !== "default" && onDeleteEnvironment && (
                          <Tooltip title="Delete environment">
                            <IconButton
                              size="small"
                              color="error"
                              onClick={() => onDeleteEnvironment(env)}
                            >
                              <Trash size={16} />
                            </IconButton>
                          </Tooltip>
                        )}
                      </Stack>
                    </FadeIn>
                  ) : (
                    <Typography variant="caption" color="text.secondary">
                      {formatDistanceToNow(new Date(env.createdAt), {
                        addSuffix: true,
                      })}
                    </Typography>
                  )}
                </Stack>
              </ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>

      {filtered.length > 5 && (
        <TablePagination
          component="div"
          count={filtered.length}
          page={page}
          rowsPerPage={rowsPerPage}
          onPageChange={(_, newPage) => setPage(newPage)}
          onRowsPerPageChange={(e) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
          rowsPerPageOptions={[5, 10, 25]}
        />
      )}
    </ResourceListShell>
  );
}

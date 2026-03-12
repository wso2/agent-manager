/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  IconButton,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Edit,
  Plus,
  Search,
  DoorClosedLocked,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, Link, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteGateway,
  useListGateways,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type GatewayResponse,
} from "@agent-management-platform/types";
import { FadeIn } from "@agent-management-platform/views";

interface AIGatewaysTableProps {
  onEditGateway?: (gateway: GatewayResponse) => void;
}

export function AIGatewaysTable({ onEditGateway }: AIGatewaysTableProps) {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const [searchQuery, setSearchQuery] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const { addConfirmation } = useConfirmationDialog();

  const {
    data: gatewaysData,
    isLoading,
    error,
    refetch,
  } = useListGateways({ orgName: orgId }, { type: "AI" });

  const { mutateAsync: deleteGateway } = useDeleteGateway();

  const gateways = useMemo(() => gatewaysData?.gateways ?? [], [gatewaysData]);

  const filteredGateways = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return gateways;
    return gateways.filter((gw) => {
      const haystack = [
        gw.name ?? "",
        gw.displayName ?? "",
        (gw as { description?: string }).description ?? "",
        gw.vhost ?? "",
      ].join(" ");
      return haystack.toLowerCase().includes(query);
    });
  }, [gateways, searchQuery]);

  useEffect(() => {
    if (page !== 0 && page * rowsPerPage >= filteredGateways.length) {
      setPage(0);
    }
  }, [filteredGateways.length, page, rowsPerPage]);

  const toolbar = (
    <Stack direction="row" spacing={1} alignItems="center">
      <Box flexGrow={1}>
        <SearchBar
          key="search-bar"
          placeholder="Search AI Gateways..."
          size="small"
          fullWidth
          value={searchQuery}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            setSearchQuery(e.target.value)
          }
          disabled={isLoading}
        />
      </Box>
      {gateways.length > 0 && (
        <Button
          component={Link}
          to={generatePath(
            absoluteRouteMap.children.org.children.gateways.children.add.path,
            { orgId },
          )}
          variant="contained"
          color="primary"
          startIcon={<Plus size={16} />}
        >
          Add AI Gateway
        </Button>
      )}
    </Stack>
  );

  if (error) {
    return (
      <>
        {toolbar}
        <ListingTable.Container>
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            sx={{ alignSelf: "stretch" }}
          >
            Failed to load gateways.{" "}
            {error instanceof Error ? error.message : "Please try again."}
          </Alert>
        </ListingTable.Container>
      </>
    );
  }

  if (isLoading) {
    return (
      <>
        {toolbar}
        <ListingTable.Container disablePaper>
          <Stack spacing={1} mt={1}>
            {Array.from({ length: 5 }).map((_, i) => (
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
                  sx={{ width: 300, flexShrink: 0 }}
                >
                  <Skeleton variant="circular" width={36} height={36} />
                  <Skeleton variant="text" width={140} height={20} />
                </Stack>
                <Skeleton
                  variant="rounded"
                  width={72}
                  height={24}
                  sx={{ flexShrink: 0 }}
                />
                <Skeleton variant="text" sx={{ flex: 1 }} height={18} />
                <Skeleton
                  variant="rounded"
                  width={100}
                  height={24}
                  sx={{ flexShrink: 0, ml: "auto" }}
                />
              </Stack>
            ))}
          </Stack>
        </ListingTable.Container>
      </>
    );
  }

  if (gateways.length === 0) {
    return (
      <Stack spacing={1}>
        {toolbar}
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<DoorClosedLocked size={64} />}
            title="No available AI gateway"
            description="Add an AI gateway to manage and monitor your AI gateway deployments."
            action={
              <Button
                component={Link}
                to={generatePath(
                  absoluteRouteMap.children.org.children.gateways.children.add
                    .path,
                  { orgId },
                )}
                variant="contained"
                startIcon={<Plus size={16} />}
              >
                Add AI Gateway
              </Button>
            }
          />
        </ListingTable.Container>
      </Stack>
    );
  }

  if (filteredGateways.length === 0) {
    return (
      <Stack spacing={1}>
        {toolbar}
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Search size={64} />}
            title="No AI Gateways found."
            description="Try a different keyword or clear the search filter."
          />
        </ListingTable.Container>
      </Stack>
    );
  }

  const paginated = filteredGateways.slice(
    page * rowsPerPage,
    page * rowsPerPage + rowsPerPage,
  );

  return (
    <>
      {toolbar}
      <Stack pt={4}>
        <ListingTable.Container disablePaper>
          <ListingTable variant="card">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell width="300px">Name</ListingTable.Cell>
                <ListingTable.Cell align="center" width="120px">
                  Status
                </ListingTable.Cell>
                <ListingTable.Cell width="140px" align="right">
                  Last Updated
                </ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {paginated.map((gateway) => {
                const displayName = gateway.displayName || gateway.name;
                const isActive =
                  gateway.status === "ACTIVE" ||
                  (gateway as { isActive?: boolean }).isActive;
                const lastUpdated = gateway.updatedAt
                  ? formatDistanceToNow(new Date(gateway.updatedAt), {
                      addSuffix: true,
                    })
                  : "—";

                return (
                  <ListingTable.Row
                    key={gateway.uuid}
                    variant="card"
                    hover
                    clickable
                    onClick={() =>
                      navigate(
                        generatePath(
                          absoluteRouteMap.children.org.children.gateways
                            .children.view.path,
                          { orgId: orgId ?? "", gatewayId: gateway.uuid },
                        ),
                      )
                    }
                    onMouseEnter={() => setHoveredId(gateway.uuid)}
                    onMouseLeave={() => setHoveredId(null)}
                    onFocus={() => setHoveredId(gateway.uuid)}
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
                          {displayName?.charAt(0)?.toUpperCase() ?? "G"}
                        </Avatar>
                        <Box>
                          <Typography variant="body2" fontWeight={500}>
                            {displayName}
                          </Typography>
                          {(gateway as { description?: string })
                            .description && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                              sx={{
                                display: "block",
                                maxWidth: 240,
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                              }}
                            >
                              {
                                (gateway as { description?: string })
                                  .description
                              }
                            </Typography>
                          )}
                        </Box>
                      </Stack>
                    </ListingTable.Cell>

                    <ListingTable.Cell align="center">
                      <Chip
                        label={isActive ? "Active" : "Inactive"}
                        size="small"
                        variant="outlined"
                        color={isActive ? "success" : "default"}
                      />
                    </ListingTable.Cell>

                    <ListingTable.Cell
                      align="right"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <Stack
                        direction="row"
                        alignItems="center"
                        spacing={1}
                        justifyContent="flex-end"
                      >
                        {hoveredId === gateway.uuid ? (
                          <FadeIn>
                            <Tooltip title="Edit">
                              <IconButton
                                size="small"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  onEditGateway?.(gateway);
                                }}
                              >
                                <Edit size={16} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Delete">
                              <IconButton
                                color="error"
                                size="small"
                                onClick={() =>
                                  addConfirmation({
                                    title: "Delete AI Gateway",
                                    description: `Are you sure you want to delete ${displayName}?`,
                                    confirmButtonText: "Delete",
                                    confirmButtonColor: "error",
                                    confirmButtonIcon: <Trash size={16} />,
                                    onConfirm: async () => {
                                      await deleteGateway({
                                        orgName: orgId ?? "",
                                        gatewayId: gateway.uuid,
                                      });
                                      refetch();
                                    },
                                  })
                                }
                              >
                                <Trash size={16} />
                              </IconButton>
                            </Tooltip>
                          </FadeIn>
                        ) : (
                          <Typography variant="caption" color="text.secondary">
                            {lastUpdated}
                          </Typography>
                        )}
                      </Stack>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                );
              })}
            </ListingTable.Body>
          </ListingTable>

          {filteredGateways.length > 5 && (
            <TablePagination
              component="div"
              count={filteredGateways.length}
              page={page}
              rowsPerPage={rowsPerPage}
              onPageChange={(_e, newPage) => setPage(newPage)}
              onRowsPerPageChange={(e) => {
                setRowsPerPage(parseInt(e.target.value, 10));
                setPage(0);
              }}
              rowsPerPageOptions={[5, 10, 25]}
            />
          )}
        </ListingTable.Container>
      </Stack>
    </>
  );
}

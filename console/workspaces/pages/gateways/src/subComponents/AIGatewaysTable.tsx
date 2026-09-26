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

import { type ChangeEvent, Fragment, useMemo, useState } from "react";
import {
  Alert,
  Avatar,
  Box,
  Chip,
  IconButton,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Edit,
  Search,
  DoorClosedLocked,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteGateway,
  useListEnvironments,
  useListGateways,
} from "@agent-management-platform/api-client";
import {
  GatewayTypeChip,
  formatRelativeTime,
  getAvatarInitials,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type GatewayResponse,
} from "@agent-management-platform/types";

interface AIGatewaysTableProps {
  onEditGateway?: (gateway: GatewayResponse) => void;
}

interface EnvironmentSection {
  key: string;
  title: string;
  gateways: GatewayResponse[];
}

const AVATAR_SX = {
  width: 28,
  height: 28,
  fontSize: 12,
  bgcolor: "primary.main",
  color: "primary.contrastText",
} as const;

const matchesQuery = (gateway: GatewayResponse, query: string) => {
  const haystack = [gateway.name ?? "", gateway.displayName ?? "", gateway.vhost ?? ""].join(" ");
  return haystack.toLowerCase().includes(query);
};

export function AIGatewaysTable({ onEditGateway }: AIGatewaysTableProps) {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const [searchQuery, setSearchQuery] = useState("");
  const { addConfirmation } = useConfirmationDialog();

  const {
    data: gatewaysData,
    isLoading: gatewaysLoading,
    error: gatewaysError,
    refetch,
  } = useListGateways({ orgName: orgId });

  const {
    data: environments,
    isLoading: environmentsLoading,
    error: environmentsError,
  } = useListEnvironments({ orgName: orgId });

  const isLoading = gatewaysLoading || environmentsLoading;
  const error = gatewaysError || environmentsError;

  const { mutateAsync: deleteGateway } = useDeleteGateway();

  const gateways = useMemo(() => gatewaysData?.gateways ?? [], [gatewaysData]);

  const filteredGateways = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return gateways;
    return gateways.filter((gw) => matchesQuery(gw, query));
  }, [gateways, searchQuery]);

  // Gateways are owned by environments (and can be mapped to more than one),
  // so group them per environment; environments with no gateways are omitted.
  const sections = useMemo<EnvironmentSection[]>(() => {
    const grouped: EnvironmentSection[] = (environments ?? [])
      .map((env) => ({
        key: env.name,
        title: env.displayName || env.name,
        gateways: filteredGateways.filter((gw) =>
          gw.environments?.some((e) => e.name === env.name),
        ),
      }))
      .filter((section) => section.gateways.length > 0);
    const unassigned = filteredGateways.filter(
      (gw) => !gw.environments?.length,
    );
    if (unassigned.length) {
      grouped.push({
        key: "__unassigned__",
        title: "Unassigned",
        gateways: unassigned,
      });
    }
    return grouped;
  }, [environments, filteredGateways]);

  const toolbar = (
    <Stack direction="row" spacing={1} alignItems="center">
      <Box flexGrow={1}>
        <SearchBar
          placeholder="Search Gateways..."
          size="small"
          fullWidth
          value={searchQuery}
          onChange={(e: ChangeEvent<HTMLInputElement>) =>
            setSearchQuery(e.target.value)
          }
          disabled={isLoading}
        />
      </Box>
    </Stack>
  );

  const renderGatewayRow = (gateway: GatewayResponse) => {
    const displayName = gateway.displayName || gateway.name;
    const isActive = gateway.status === "ACTIVE";
    const lastUpdated = formatRelativeTime(gateway.updatedAt);

    return (
      <ListingTable.Row
        key={gateway.uuid}
        variant="card"
        hover
        clickable
        onClick={() =>
          navigate(
            generatePath(
              absoluteRouteMap.children.org.children.gateways.children.view
                .path,
              { orgId: orgId ?? "", gatewayId: gateway.uuid },
            ),
          )
        }
      >
        <ListingTable.Cell>
          <ListingTable.CellIcon
            icon={
              <Avatar sx={AVATAR_SX}>
                {getAvatarInitials(displayName, { fallback: "G", maxChars: 1 })}
              </Avatar>
            }
            primary={displayName}
          />
        </ListingTable.Cell>

        <ListingTable.Cell align="left">
          <GatewayTypeChip type={gateway.gatewayType} />
        </ListingTable.Cell>

        <ListingTable.Cell align="center">
          <Chip
            label={isActive ? "Active" : "Inactive"}
            size="small"
            variant="outlined"
            color={isActive ? "success" : "default"}
          />
        </ListingTable.Cell>

        <ListingTable.Cell align="right">
          <Typography variant="caption" color="text.secondary">
            {lastUpdated}
          </Typography>
        </ListingTable.Cell>

        <ListingTable.Cell align="center">
          <ListingTable.RowActions visibility="hover">
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
                onClick={(e) => {
                  e.stopPropagation();
                  addConfirmation({
                    analytics: { entity: "gateway", action: "delete" },
                    title: "Delete Gateway",
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
                  });
                }}
              >
                <Trash size={16} />
              </IconButton>
            </Tooltip>
          </ListingTable.RowActions>
        </ListingTable.Cell>
      </ListingTable.Row>
    );
  };

  // The body renders below a toolbar that stays mounted at a stable tree
  // position across all states — swapping the toolbar's parent structure
  // between renders would remount the search input and drop its focus.
  const renderBody = () => {
    if (error) {
      const failedResource = gatewaysError ? "gateways" : "environments";
      return (
        <ListingTable.Container>
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            sx={{ alignSelf: "stretch" }}
          >
            Failed to load {failedResource}.{" "}
            {error instanceof Error ? error.message : "Please try again."}
          </Alert>
        </ListingTable.Container>
      );
    }

    if (isLoading) {
      return (
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
      );
    }

    if (gateways.length === 0) {
      return (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<DoorClosedLocked size={64} />}
            title="No available gateway"
            description="No gateways have been configured yet."
          />
        </ListingTable.Container>
      );
    }

    if (filteredGateways.length === 0) {
      return (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Search size={64} />}
            title="No Gateways found."
            description="Try a different keyword or clear the search filter."
          />
        </ListingTable.Container>
      );
    }

    return (
      <Stack pt={3}>
        <ListingTable.Container disablePaper>
          <ListingTable variant="card">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell align="left" width="120px">
                  Type
                </ListingTable.Cell>
                <ListingTable.Cell align="center" width="120px">
                  Status
                </ListingTable.Cell>
                <ListingTable.Cell align="right" width="140px">
                  Last Updated
                </ListingTable.Cell>
                <ListingTable.Cell align="center" width="96px" />
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {sections.map((section) => (
                <Fragment key={section.key}>
                  <ListingTable.Row>
                    {/* "&&" outranks the table's descendant padding rule on
                        .MuiTableCell-root, which beats a plain sx padding. */}
                    <ListingTable.Cell
                      colSpan={5}
                      sx={{ "&&": { border: 0, p: 0 } }}
                    >
                      <Typography variant="overline" color="text.secondary">
                        {section.title}
                      </Typography>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                  {section.gateways.map(renderGatewayRow)}
                </Fragment>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      </Stack>
    );
  };

  return (
    <Stack spacing={1}>
      {toolbar}
      {renderBody()}
    </Stack>
  );
}

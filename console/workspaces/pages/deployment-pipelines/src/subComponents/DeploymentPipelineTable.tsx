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

import { type ChangeEvent, useEffect, useMemo, useState } from "react";
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
import { AlertTriangle, ArrowRight, Edit, GitBranch, Plus, Search, Trash } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { useParams } from "react-router-dom";
import {
  useDeleteDeploymentPipeline,
  useListDeploymentPipelines,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import type { DeploymentPipelineResponse } from "@agent-management-platform/types";
import { getIsolationTierMeta, useConfirmationDialog } from "@agent-management-platform/shared-component";
import { FadeIn } from "@agent-management-platform/views";
import { validatePromotionChain } from "../utils/validatePromotionChain";

interface DeploymentPipelineTableProps {
  onEditPipeline?: (pipeline: DeploymentPipelineResponse) => void;
  onCreatePipeline?: () => void;
}

interface EnvInfo { displayName?: string; isProduction: boolean; isolationTier?: string; }

function PromotionChainCell({
  pipeline,
  envMap,
}: {
  pipeline: DeploymentPipelineResponse;
  envMap: Map<string, EnvInfo>;
}) {
  const validation = useMemo(
    () => validatePromotionChain(pipeline.promotionPaths),
    [pipeline.promotionPaths],
  );

  if (!validation.valid) {
    return (
      <Tooltip title={validation.error}>
        <Stack direction="row" spacing={0.5} alignItems="center" sx={{ cursor: "default" }}>
          <AlertTriangle size={14} color="var(--oxygen-palette-warning-main)" />
          <Typography variant="body2" color="warning.main">
            Invalid paths
          </Typography>
        </Stack>
      </Tooltip>
    );
  }

  const chain = validation.chain ?? [];
  return (
    <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap">
      {chain.map((envName, index) => {
        const env = envMap.get(envName);
        const tierMeta = getIsolationTierMeta(env?.isolationTier);
        const TierIcon = tierMeta.icon;
        return (
          <Stack key={envName} direction="row" spacing={0.5} alignItems="center">
            <Tooltip title={tierMeta.fullLabel}>
              <Chip
                icon={<TierIcon size={14} />}
                label={
                  <Typography component="span">
                    <Typography variant="body2" component="span">{env?.displayName ?? envName}</Typography>
                    {env?.isProduction && (
                      <Typography component="span" variant="caption"> (Production)</Typography>
                    )}
                  </Typography>
                }
                variant="outlined"
                sx={{ "& .MuiChip-icon": { color: tierMeta.iconColor } }}
              />
            </Tooltip>
            {index < chain.length - 1 && <ArrowRight size={12} />}
          </Stack>
        );
      })}
    </Stack>
  );
}

export function DeploymentPipelineTable(
  { onEditPipeline, onCreatePipeline }: DeploymentPipelineTableProps,
) {
  const { orgId } = useParams<{ orgId: string }>();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);
  const [hoveredId, setHoveredId] = useState<string | null>(null);

  const { data, isLoading, error } = useListDeploymentPipelines({ orgName: orgId });
  const { data: environments } = useListEnvironments({ orgName: orgId });
  const { addConfirmation } = useConfirmationDialog();
  const { mutate: deletePipeline } = useDeleteDeploymentPipeline();

  const envMap = useMemo(() => {
    const map = new Map<string, EnvInfo>();
    (environments ?? []).forEach((e) => {
      map.set(e.name, {
        displayName: e.displayName,
        isProduction: e.isProduction,
        isolationTier: e.isolationTier,
      });
    });
    return map;
  }, [environments]);

  const pipelines = useMemo(
    () => data?.deploymentPipelines ?? [],
    [data?.deploymentPipelines],
  );

  const filtered = useMemo(() => {
    const q = search.toLowerCase();
    if (!q) return pipelines;
    return pipelines.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        p.displayName.toLowerCase().includes(q),
    );
  }, [pipelines, search]);

  useEffect(() => {
    if (page !== 0 && page * rowsPerPage >= filtered.length) setPage(0);
  }, [filtered.length, page, rowsPerPage]);

  const toolbar = (
    <Stack direction="row" spacing={1} alignItems="center">
      <Box flexGrow={1}>
        <SearchBar
          value={search}
          onChange={(e: ChangeEvent<HTMLInputElement>) => {
            setSearch(e.target.value);
            setPage(0);
          }}
          placeholder="Search pipelines..."
          size="small"
          fullWidth
          disabled={isLoading}
        />
      </Box>
      {onCreatePipeline && pipelines.length > 0 && (
        <Button
          variant="contained"
          startIcon={<Plus size={16} />}
          onClick={onCreatePipeline}
        >
          Create Pipeline
        </Button>
      )}
    </Stack>
  );

  if (error) {
    return (
      <Stack spacing={1}>
        {toolbar}
        <ListingTable.Container>
          <Alert severity="error" icon={<AlertTriangle size={18} />} sx={{ alignSelf: "stretch" }}>
            Failed to load pipelines.{" "}
            {error instanceof Error ? error.message : "Please try again."}
          </Alert>
        </ListingTable.Container>
      </Stack>
    );
  }

  if (isLoading) {
    return (
      <>
        {toolbar}
        <ListingTable.Container disablePaper>
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
                <Stack direction="row" alignItems="center" spacing={1.5} sx={{ width: 280, flexShrink: 0 }}>
                  <Skeleton variant="circular" width={36} height={36} />
                  <Skeleton variant="text" width={140} height={20} />
                </Stack>
                <Stack direction="row" spacing={0.5} sx={{ flex: 1 }}>
                  <Skeleton variant="rounded" width={72} height={24} />
                  <Skeleton variant="rounded" width={72} height={24} />
                </Stack>
                <Skeleton variant="rounded" width={100} height={24} sx={{ flexShrink: 0, ml: "auto" }} />
              </Stack>
            ))}
          </Stack>
        </ListingTable.Container>
      </>
    );
  }

  if (pipelines.length === 0) {
    return (
      <Stack spacing={1}>
        {toolbar}
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<GitBranch size={64} />}
            title="No deployment pipelines yet"
            description="Deployment pipelines define the promotion path for your agents across environments."
            action={
              onCreatePipeline && (
                <Button
                  variant="contained"
                  startIcon={<Plus size={16} />}
                  onClick={onCreatePipeline}
                >
                  Create Pipeline
                </Button>
              )
            }
          />
        </ListingTable.Container>
      </Stack>
    );
  }

  if (filtered.length === 0) {
    return (
      <Stack spacing={1}>
        {toolbar}
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Search size={64} />}
            title="No pipelines match your search"
            description="Try a different keyword or clear the search filter."
          />
        </ListingTable.Container>
      </Stack>
    );
  }

  const paginated = filtered.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);

  return (
    <>
      {toolbar}
      <Stack pt={4}>
        <ListingTable.Container disablePaper>
          <ListingTable variant="card">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell width="280px">Pipeline</ListingTable.Cell>
                <ListingTable.Cell>Promotion Chain</ListingTable.Cell>
                <ListingTable.Cell align="right" width="160px">Created</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {paginated.map((pipeline) => (
                <ListingTable.Row
                  key={pipeline.name}
                  variant="card"
                  hover
                  onMouseEnter={() => setHoveredId(pipeline.name)}
                  onMouseLeave={() => setHoveredId(null)}
                  onFocus={() => setHoveredId(pipeline.name)}
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
                        {pipeline.displayName?.charAt(0)?.toUpperCase() ?? "P"}
                      </Avatar>
                      <Typography
                        variant="body2"
                        fontWeight={500}
                        noWrap
                      >
                        {pipeline.displayName}
                      </Typography>
                    </Stack>
                  </ListingTable.Cell>

                  <ListingTable.Cell>
                    <PromotionChainCell pipeline={pipeline} envMap={envMap} />
                  </ListingTable.Cell>

                  <ListingTable.Cell align="right" onClick={(e) => e.stopPropagation()}>
                    <Stack direction="row" justifyContent="flex-end" alignItems="center" spacing={1}>
                      {hoveredId === pipeline.name ? (
                        <FadeIn>
                          <Stack direction="row" spacing={0.5}>
                            <Tooltip title="Edit pipeline">
                              <IconButton size="small" onClick={() => onEditPipeline?.(pipeline)}>
                                <Edit size={16} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Delete pipeline">
                              <IconButton
                                size="small"
                                color="error"
                                onClick={() =>
                                  addConfirmation({
                                    analytics: { entity: "deployment-pipeline", action: "delete" },
                                    title: "Delete Deployment Pipeline",
                                    description: `Are you sure you want to delete "${pipeline.displayName}"? This action cannot be undone.`,
                                    confirmButtonText: "Delete",
                                    confirmButtonColor: "error",
                                    confirmButtonIcon: <Trash size={16} />,
                                    onConfirm: () =>
                                      deletePipeline({
                                        orgName: orgId,
                                        pipelineName: pipeline.name,
                                      }),
                                  })
                                }
                              >
                                <Trash size={16} />
                              </IconButton>
                            </Tooltip>
                          </Stack>
                        </FadeIn>
                      ) : (
                        <Typography variant="caption" color="text.secondary">
                          {formatDistanceToNow(new Date(pipeline.createdAt), { addSuffix: true })}
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
        </ListingTable.Container>
      </Stack>
    </>
  );
}

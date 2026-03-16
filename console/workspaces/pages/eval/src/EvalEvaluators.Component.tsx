/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { PageLayout } from "@agent-management-platform/views";
import {
  Alert,
  Box,
  Button,
  CardContent,
  CardHeader,
  Chip,
  ListingTable,
  SearchBar,
  Skeleton,
  Snackbar,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Pencil,
  Plus,
  CircleIcon,
  Search as SearchIcon,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type EvaluatorResponse,
} from "@agent-management-platform/types";
import {
  useListEvaluators,
  useDeleteCustomEvaluator,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import debounce from "lodash/debounce";

type SourceFilter = "all" | "builtin" | "custom";

const sourceFilterOptions: { label: string; value: SourceFilter }[] = [
  { label: "All", value: "all" },
  { label: "Built-in", value: "builtin" },
  { label: "Custom", value: "custom" },
];

function getSourceLabel(evaluator: EvaluatorResponse): string {
  return evaluator.isBuiltin ? "Built-in" : "Custom";
}

function getSourceColor(
  evaluator: EvaluatorResponse,
):
  | "default"
  | "primary"
  | "secondary"
  | "success"
  | "warning"
  | "error"
  | "info" {
  return evaluator.isBuiltin ? "default" : "info";
}

export const EvalEvaluatorsComponent: React.FC = () => {
  const { agentId, orgId, projectId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();

  const [sourceFilter, setSourceFilter] = useState<SourceFilter>("all");
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(12);

  const {
    data,
    isLoading,
    error: evaluatorsError,
  } = useListEvaluators(
    { orgName: orgId },
    {
      limit: rowsPerPage,
      offset: page * rowsPerPage,
      search: debouncedSearch.trim() || undefined,
      source: sourceFilter === "all" ? undefined : sourceFilter,
    },
  );

  const evaluators = useMemo(() => data?.evaluators ?? [], [data]);
  const totalItems = data?.total ?? evaluators.length;

  const {
    mutate: deleteEvaluator,
    error: deleteError,
    reset: resetDeleteError,
  } = useDeleteCustomEvaluator();

  const { addConfirmation } = useConfirmationDialog();

  const debouncedSetSearch = useMemo(
    () =>
      debounce((value: string) => {
        setDebouncedSearch(value);
        setPage(0);
      }, 300),
    [],
  );

  useEffect(
    () => () => {
      debouncedSetSearch.cancel();
    },
    [debouncedSetSearch],
  );

  const handleDelete = useCallback(
    (evaluator: EvaluatorResponse) => {
      addConfirmation({
        title: "Delete Evaluator",
        description: `Are you sure you want to delete "${evaluator.displayName}"? This action cannot be undone.`,
        confirmButtonText: "Delete",
        confirmButtonColor: "error",
        confirmButtonIcon: <Trash size={16} />,
        onConfirm: () => {
          deleteEvaluator({
            orgName: orgId!,
            identifier: evaluator.identifier,
          });
        },
      });
    },
    [deleteEvaluator, orgId, addConfirmation],
  );

  const evaluatorsRouteMap = agentId
    ? absoluteRouteMap.children.org.children.projects.children.agents.children
        .evaluation.children.evaluators
    : absoluteRouteMap.children.org.children.projects.children.evaluators;

  const routeParams = agentId
    ? { orgId, projectId, agentId }
    : { orgId, projectId };

  return (
    <>
    <PageLayout
      title="Evaluators"
      disableIcon
      actions={
        <Button
          variant="contained"
          component={Link}
          to={generatePath(
            evaluatorsRouteMap.children.create.path,
            routeParams,
          )}
          startIcon={<Plus />}
          color="primary"
        >
          Create Evaluator
        </Button>
      }
    >
      <Stack spacing={2}>
        <Stack
          direction="row"
          spacing={1}
          alignItems="center"
          justifyContent="space-between"
          flexWrap="wrap"
          useFlexGap
        >
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
            {sourceFilterOptions.map((option) => (
              <Chip
                key={option.value}
                label={option.label}
                variant={sourceFilter === option.value ? "filled" : "outlined"}
                color={sourceFilter === option.value ? "primary" : "default"}
                onClick={() => {
                  setSourceFilter(option.value);
                  setPage(0);
                }}
              />
            ))}
          </Stack>
          <SearchBar
            placeholder="Search evaluators"
            size="small"
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              debouncedSetSearch(event.target.value);
            }}
            disabled={isLoading}
          />
        </Stack>

        {evaluatorsError && (
          <Alert severity="error">
            {evaluatorsError instanceof Error
              ? evaluatorsError.message
              : "Failed to load evaluators"}
          </Alert>
        )}


        {isLoading && (
          <Stack direction="row" gap={1}>
            <Skeleton variant="rounded" height={180} width="100%" />
            <Skeleton variant="rounded" height={180} width="100%" />
            <Skeleton variant="rounded" height={180} width="100%" />
            <Skeleton variant="rounded" height={180} width="100%" />
          </Stack>
        )}

        {!isLoading &&
          !evaluatorsError &&
          evaluators.length === 0 &&
          !search.trim() && (
            <ListingTable.Container sx={{ my: 3 }}>
              <ListingTable.EmptyState
                illustration={<CircleIcon size={64} />}
                title="No evaluators yet"
                description="Create a custom evaluator or browse built-in evaluators."
              />
            </ListingTable.Container>
          )}

        {evaluators.length === 0 && !isLoading && search.trim() && (
          <ListingTable.Container sx={{ my: 3 }}>
            <ListingTable.EmptyState
              illustration={<SearchIcon size={64} />}
              title="No evaluators match your search"
              description="Try a different keyword or clear the search filter."
            />
          </ListingTable.Container>
        )}

        {evaluators.length > 0 && (
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: {
                xs: "repeat(auto-fill, minmax(260px, 1fr))",
                md: "repeat(auto-fill, minmax(300px, 1fr))",
              },
              gap: 2,
            }}
          >
            {evaluators.map((evaluator) => (
              <Box
                key={evaluator.identifier}
                role="button"
                tabIndex={0}
                onClick={() =>
                  navigate(
                    generatePath(evaluatorsRouteMap.children.view.path, {
                      ...routeParams,
                      evaluatorId: evaluator.identifier,
                    }),
                  )
                }
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    navigate(
                      generatePath(evaluatorsRouteMap.children.view.path, {
                        ...routeParams,
                        evaluatorId: evaluator.identifier,
                      }),
                    );
                  }
                }}
                sx={{
                  border: 1,
                  borderColor: "divider",
                  borderRadius: 2,
                  p: 0,
                  display: "flex",
                  flexDirection: "column",
                  cursor: "pointer",
                  "&:hover": {
                    borderColor: "primary.main",
                    boxShadow: 1,
                  },
                }}
              >
                <CardHeader
                  title={
                    <Stack direction="column" spacing={1}>
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Typography
                          variant="h6"
                          textOverflow="ellipsis"
                          overflow="hidden"
                          whiteSpace="nowrap"
                          maxWidth="70%"
                        >
                          {evaluator.displayName}
                        </Typography>
                        <Chip
                          label={getSourceLabel(evaluator)}
                          size="small"
                          variant="filled"
                          color={getSourceColor(evaluator)}
                        />
                        {evaluator.level && (
                          <Chip
                            label={
                              evaluator.level.charAt(0).toUpperCase() +
                              evaluator.level.slice(1)
                            }
                            size="small"
                            variant="outlined"
                            color="primary"
                          />
                        )}
                      </Stack>
                      {(() => {
                        const tags = evaluator.tags ?? [];
                        return tags.length > 0 ? (
                          <Stack
                            direction="row"
                            spacing={1}
                            alignItems="center"
                          >
                            {tags.slice(0, 3).map((tag) => (
                              <Chip
                                key={tag}
                                size="small"
                                label={tag}
                                variant="outlined"
                              />
                            ))}
                            {tags.length > 3 && (
                              <Tooltip title={tags.join(", ")} placement="top">
                                <Typography
                                  variant="caption"
                                  color="text.secondary"
                                >
                                  {`+${tags.length - 3} more`}
                                </Typography>
                              </Tooltip>
                            )}
                          </Stack>
                        ) : null;
                      })()}
                    </Stack>
                  }
                />
                <CardContent sx={{ flexGrow: 1 }}>
                  <Typography variant="caption" color="text.secondary">
                    {evaluator.description}
                  </Typography>
                </CardContent>
                {!evaluator.isBuiltin && (
                  <Stack
                    direction="row"
                    justifyContent="flex-end"
                    spacing={1}
                    px={2}
                    pb={1}
                    onClick={(e) => e.stopPropagation()}
                  >
                    <Button
                      size="small"
                      variant="text"
                      startIcon={<Pencil size={14} />}
                      onClick={() =>
                        navigate(
                          generatePath(
                            evaluatorsRouteMap.children.view.path,
                            {
                              ...routeParams,
                              evaluatorId: evaluator.identifier,
                            },
                          ),
                          { state: { edit: true } },
                        )
                      }
                    >
                      Edit
                    </Button>
                    <Button
                      size="small"
                      variant="text"
                      color="error"
                      startIcon={<Trash size={14} />}
                      onClick={() => handleDelete(evaluator)}
                    >
                      Delete
                    </Button>
                  </Stack>
                )}
              </Box>
            ))}
          </Box>
        )}

        {totalItems > rowsPerPage && (
          <TablePagination
            component="div"
            count={totalItems}
            page={page}
            rowsPerPage={rowsPerPage}
            onPageChange={(_event, newPage) => setPage(newPage)}
            onRowsPerPageChange={(event) => {
              const next = parseInt(event.target.value, 10);
              setRowsPerPage(next);
              setPage(0);
            }}
            rowsPerPageOptions={[6, 12, 24]}
          />
        )}
      </Stack>
    </PageLayout>
    <Snackbar
      open={!!deleteError}
      autoHideDuration={6000}
      onClose={resetDeleteError}
      anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
    >
      <Alert onClose={resetDeleteError} severity="error">
        {(deleteError as { message?: string })?.message ||
          "Failed to delete evaluator"}
      </Alert>
    </Snackbar>
    </>
  );
};

export default EvalEvaluatorsComponent;

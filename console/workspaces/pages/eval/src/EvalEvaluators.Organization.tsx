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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type MouseEvent,
} from "react";
import {
  getErrorMessage,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  Divider,
  Form,
  IconButton,
  InputAdornment,
  ListingTable,
  ListItemText,
  MenuItem,
  SearchBar,
  Select,
  Skeleton,
  Snackbar,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Edit as EditIcon,
  Plus,
  CircleIcon,
  Filter,
  Search as SearchIcon,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type EvaluatorLevel,
  type EvaluatorResponse,
} from "@agent-management-platform/types";
import {
  useListEvaluators,
  useDeleteCustomEvaluator,
} from "@agent-management-platform/api-client";
import debounce from "lodash/debounce";
import { SectionErrorBoundary } from "./subComponents/SectionErrorBoundary";

type EvaluatorSource = "builtin" | "custom";

const sourceFilterOptions: { label: string; value: EvaluatorSource }[] = [
  { label: "Built-in", value: "builtin" },
  { label: "Custom", value: "custom" },
];

const sourceLabel = (value: EvaluatorSource) =>
  sourceFilterOptions.find((option) => option.value === value)?.label ?? value;

const LEVEL_DISPLAY: Record<EvaluatorLevel, { label: string }> = {
  trace: { label: "Trace" },
  agent: { label: "Agent" },
  llm: { label: "LLM" },
};

const METHOD_LABELS: Record<string, string> = {
  "rule-based": "Rule-based",
  "code": "Rule-based",
  "llm-judge": "LLM Judge",
};

export const EvalEvaluatorsOrganization: React.FC = () => {
  const { orgId } = useParams<{
    orgId: string;
  }>();
  const navigate = useNavigate();

  const [selectedSources, setSelectedSources] = useState<EvaluatorSource[]>([]);
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
      // The API takes a single source; a multi-select of both (or none) means
      // "no filter", and exactly one selection narrows to that source.
      source: selectedSources.length === 1 ? selectedSources[0] : undefined,
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
      }, 1000),
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
        analytics: { entity: "evaluator", action: "delete" },
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

  const evaluatorsRouteMap = absoluteRouteMap.children.org.children.evaluators;

  const routeParams = { orgId };

  return (
    <>
      <PageLayout title="Evaluators" disableIcon>
        <Stack spacing={2}>
          <Stack
            direction="row"
            spacing={1}
            alignItems="center"
            flexWrap="wrap"
            useFlexGap
          >
            <Box flexGrow={1}>
              <SearchBar
                placeholder="Search evaluators"
                size="small"
                fullWidth
                value={search}
                onChange={(event) => {
                  setSearch(event.target.value);
                  debouncedSetSearch(event.target.value);
                }}
              />
            </Box>
            <Select
              size="small"
              variant="outlined"
              multiple
              value={selectedSources}
              onChange={(event) => {
                setSelectedSources(event.target.value as EvaluatorSource[]);
                setPage(0);
              }}
              displayEmpty
              renderValue={(selected) =>
                (selected as EvaluatorSource[]).length === 0
                  ? sourceFilterOptions.map((option) => sourceLabel(option.value)).join(", ")
                  : (selected as EvaluatorSource[]).map(sourceLabel).join(", ")
              }
              startAdornment={
                <InputAdornment position="start">
                  <Filter size={16} />
                </InputAdornment>
              }
              sx={{ minWidth: 180 }}
            >
              {sourceFilterOptions.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  <Checkbox
                    checked={selectedSources.includes(option.value)}
                    size="small"
                  />
                  <ListItemText primary={option.label} />
                </MenuItem>
              ))}
            </Select>
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
          </Stack>

          {evaluatorsError ? (
            <Alert severity="error">
              {getErrorMessage(evaluatorsError) || "Failed to load evaluators"}
            </Alert>
          ) : null}

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
            <SectionErrorBoundary fallbackMessage="Failed to render evaluator list. Click Retry to try again.">
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
                {evaluators.map((evaluator) => {
                  const viewPath = generatePath(
                    evaluatorsRouteMap.children.view.path,
                    { ...routeParams, evaluatorId: evaluator.identifier },
                  );
                  const handleEditClick = (
                    event: MouseEvent<HTMLButtonElement>,
                  ) => {
                    event.preventDefault();
                    event.stopPropagation();
                    navigate(viewPath, { state: { edit: true } });
                  };
                  const handleDeleteClick = (
                    event: MouseEvent<HTMLButtonElement>,
                  ) => {
                    event.preventDefault();
                    event.stopPropagation();
                    handleDelete(evaluator);
                  };
                  const allTags = evaluator.tags ?? [];
                  const methodTag = allTags.find((tag) => tag in METHOD_LABELS);
                  // `type` is authoritative for custom evaluators; built-ins have none, so fall back to their tag.
                  const methodLabel =
                    evaluator.type === "code"
                      ? "Rule-based"
                      : evaluator.type === "llm_judge"
                        ? "LLM Judge"
                        : methodTag
                          ? METHOD_LABELS[methodTag]
                          : undefined;
                  const tags = allTags.filter((tag) => tag !== methodTag);
                  const desc = evaluator.description ?? "";
                  const levelDisplay = evaluator.level
                    ? LEVEL_DISPLAY[evaluator.level]
                    : undefined;
                  const classificationChips: Array<{
                    key: string;
                    label: string;
                    color: "default" | "primary" | "info";
                  }> = [];
                  if (!evaluator.isBuiltin) {
                    classificationChips.push({ key: "origin", label: "Custom", color: "default" });
                  }
                  if (levelDisplay) {
                    classificationChips.push({ key: "level", label: levelDisplay.label, color: "primary" });
                  }
                  if (methodLabel) {
                    classificationChips.push({ key: "method", label: methodLabel, color: "info" });
                  }
                  const descEl = (
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{
                        display: "-webkit-box",
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: "vertical",
                        overflow: "hidden",
                        mb: 1,
                      }}
                    >
                      {desc}
                    </Typography>
                  );
                  return (
                    <Link
                      key={evaluator.identifier}
                      to={viewPath}
                      style={{ textDecoration: "none" }}
                    >
                      <Form.CardButton
                        sx={{
                          width: "100%",
                          textAlign: "left",
                          textDecoration: "none",
                          height: 200,
                          display: "flex",
                          flexDirection: "column",
                          justifyContent: "flex-start",
                        }}
                      >
                        <Form.CardHeader
                          sx={{
                            width: "100%",
                            minWidth: 0,
                            overflow: "hidden",
                            "& .MuiCardHeader-content": { minWidth: 0 },
                          }}
                          title={
                            <Form.Stack
                              direction="column"
                              spacing={1}
                              sx={{ minWidth: 0, width: "100%" }}
                            >
                              <Tooltip title={evaluator.displayName} placement="top">
                                <Typography
                                  variant="h6"
                                  noWrap
                                  sx={{ minWidth: 0 }}
                                >
                                  {evaluator.displayName}
                                </Typography>
                              </Tooltip>

                              {classificationChips.length > 0 && (
                                <Stack
                                  direction="row"
                                  spacing={1}
                                  alignItems="center"
                                  sx={{ opacity: 0.85 }}
                                >
                                  {classificationChips.map((chip, index) => (
                                    <Stack key={chip.key} direction="row" spacing={1} alignItems="center">
                                      {index > 0 && (
                                        <Divider orientation="vertical" flexItem sx={{ height: 16, alignSelf: "center" }} />
                                      )}
                                      <Chip
                                        label={chip.label}
                                        size="small"
                                        variant="outlined"
                                        color={chip.color}
                                        sx={{
                                          height: 20,
                                          fontSize: "0.7rem",
                                          "& .MuiChip-label": { px: 0.75 },
                                        }}
                                      />
                                    </Stack>
                                  ))}
                                </Stack>
                              )}
                            </Form.Stack>
                          }
                        />
                        <Form.CardContent
                          sx={{
                            width: "100%",
                            display: "flex",
                            flexDirection: "column",
                            justifyContent: "space-between",
                            flexGrow: 1,
                            minHeight: 0,
                            // Pinned to avoid MUI's `:last-child` padding bump, which otherwise toggles on first hover and shifts this row.
                            pb: 2,
                            "&.MuiCardContent-root:last-child": { pb: 2 },
                          }}
                        >
                          <Tooltip title={desc} placement="top">
                            {descEl}
                          </Tooltip>

                          <Stack
                            direction="row"
                            alignItems="center"
                            justifyContent="space-between"
                            sx={{ width: "100%", minWidth: 0, pt: 1 }}
                          >
                            {tags.length > 0 ? (
                              <Stack
                                direction="row"
                                spacing={0.75}
                                alignItems="center"
                                sx={{ minWidth: 0, overflow: "hidden", flexWrap: "nowrap" }}
                              >
                                {tags.slice(0, 2).map((tag) => (
                                  <Chip
                                    key={tag}
                                    label={tag}
                                    size="small"
                                    variant="filled"
                                    color="default"
                                    sx={{ flexShrink: 0 }}
                                  />
                                ))}
                                {tags.length > 2 && (
                                  <Tooltip title={tags.join(", ")} placement="top">
                                    <Typography
                                      variant="caption"
                                      color="text.secondary"
                                      sx={{ flexShrink: 0, whiteSpace: "nowrap" }}
                                    >
                                      {`+${tags.length - 2} more`}
                                    </Typography>
                                  </Tooltip>
                                )}
                              </Stack>
                            ) : (
                              <span />
                            )}
                            {!evaluator.isBuiltin && (
                              <Form.DisappearingCardButtonContent>
                                <Tooltip title="Edit">
                                  <IconButton
                                    size="small"
                                    aria-label="Edit"
                                    onClick={handleEditClick}
                                  >
                                    <EditIcon size={16} />
                                  </IconButton>
                                </Tooltip>
                                <Tooltip title="Delete">
                                  <IconButton
                                    size="small"
                                    color="error"
                                    aria-label="Delete"
                                    onClick={handleDeleteClick}
                                  >
                                    <Trash size={16} />
                                  </IconButton>
                                </Tooltip>
                              </Form.DisappearingCardButtonContent>
                            )}
                          </Stack>
                        </Form.CardContent>
                      </Form.CardButton>
                    </Link>
                  );
                })}
              </Box>
            </SectionErrorBoundary>
          )}

          {totalItems > 6 && (
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

export default EvalEvaluatorsOrganization;

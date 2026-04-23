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

import {
  Alert,
  Avatar,
  Box,
  Button,
  CardContent,
  CardHeader,
  Chip,
  Form,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { getErrorMessage } from "@agent-management-platform/shared-component";
import {
  Check,
  CircleIcon,
  Search as SearchIcon,
  Settings,
} from "@wso2/oxygen-ui-icons-react";
import type {
  EvaluatorResponse,
  MonitorEvaluator,
  MonitorLLMProviderRef,
} from "@agent-management-platform/types";
import {
  useListEvaluators,
  useListLLMProviders,
} from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";
import { useMemo, useState, useCallback, useEffect } from "react";
import debounce from "lodash/debounce";
import EvaluatorDetailsDrawer from "./EvaluatorDetailsDrawer";
import { MonitorLLMProviderDrawer } from "./MonitorLLMProviderDrawer";

const toSlug = (value: string): string =>
  value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60);

const getEvaluatorIdentifier = (evaluator: {
  identifier?: string;
  displayName: string;
}): string => evaluator.identifier ?? toSlug(evaluator.displayName);

interface SelectPresetMonitorsProps {
  selectedEvaluators: MonitorEvaluator[];
  onToggleEvaluator: (evaluator: EvaluatorResponse) => void;
  onSaveEvaluatorConfig: (
    evaluator: EvaluatorResponse,
    config: Record<string, unknown>,
  ) => void;
  llmProvider?: MonitorLLMProviderRef;
  onLLMProviderChange: (provider: MonitorLLMProviderRef | undefined) => void;
  onHasLLMJudgeChange: (hasLLMJudge: boolean) => void;
  error?: string;
  llmProviderError?: string;
}

export function SelectPresetMonitors({
  selectedEvaluators,
  onToggleEvaluator,
  onSaveEvaluatorConfig,
  llmProvider,
  onLLMProviderChange,
  onHasLLMJudgeChange,
  error,
  llmProviderError,
}: SelectPresetMonitorsProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(12);

  const {
    data,
    isLoading,
    error: evaluatorsError,
  } = useListEvaluators(
    {
      orgName: orgId,
    },
    {
      limit: rowsPerPage,
      offset: page * rowsPerPage,
      search: debouncedSearch.trim() || undefined,
    },
  );
  const evaluators = useMemo(() => data?.evaluators ?? [], [data]);
  const selectedProviderName = llmProvider?.providerName;

  const { data: providersData } = useListLLMProviders({ orgName: orgId });
  const providerDisplayName = useMemo(
    () =>
      providersData?.providers.find((p) => p.id === selectedProviderName)
        ?.name ?? selectedProviderName,
    [providersData, selectedProviderName],
  );

  const [llmJudgeIds, setLlmJudgeIds] = useState<Set<string>>(() => new Set());
  const [providerDrawerOpen, setProviderDrawerOpen] = useState(false);
  const [pendingEvaluator, setPendingEvaluator] = useState<EvaluatorResponse | null>(null);
  const [drawerEvaluator, setDrawerEvaluator] =
    useState<EvaluatorResponse | null>(null);

  const handleProviderChange = useCallback(
    (name: string | undefined) => {
      onLLMProviderChange(name ? { providerName: name } : undefined);
      if (name && pendingEvaluator) {
        setDrawerEvaluator(pendingEvaluator);
        setPendingEvaluator(null);
      }
    },
    [onLLMProviderChange, pendingEvaluator],
  );

  const selectedEvaluatorNames = useMemo(
    () => selectedEvaluators.map((item) => getEvaluatorIdentifier(item)),
    [selectedEvaluators],
  );

  // Cross-reference selected evaluators with the current page to detect LLM judges
  // even before the user opens/confirms them through the drawer (e.g. edit mode).
  const llmJudgeIdsOnPage = useMemo(
    () =>
      new Set(
        evaluators
          .filter((e) => e.type === "llm_judge")
          .map(getEvaluatorIdentifier),
      ),
    [evaluators],
  );
  const hasLLMJudge = useMemo(
    () =>
      selectedEvaluatorNames.some((id) => llmJudgeIdsOnPage.has(id)) ||
      llmJudgeIds.size > 0,
    [selectedEvaluatorNames, llmJudgeIdsOnPage, llmJudgeIds],
  );
  useEffect(() => {
    onHasLLMJudgeChange(hasLLMJudge);
  }, [hasLLMJudge, onHasLLMJudgeChange]);

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

  const totalItems = data?.total ?? evaluators.length;

  const selectedChipEvaluators = useMemo(() => {
    const byId = new Map<string, MonitorEvaluator>();
    selectedEvaluators.forEach((item) => {
      byId.set(getEvaluatorIdentifier(item), item);
    });
    return Array.from(byId.values());
  }, [selectedEvaluators]);

  const handleOpenDrawer = useCallback(
    (evaluator: EvaluatorResponse) => {
      if (evaluator.type === "llm_judge" && !selectedProviderName) {
        setPendingEvaluator(evaluator);
        setProviderDrawerOpen(true);
        return;
      }
      setDrawerEvaluator(evaluator);
    },
    [selectedProviderName],
  );

  const handleCloseDrawer = useCallback(() => {
    setDrawerEvaluator(null);
  }, []);

  const drawerIdentifier = drawerEvaluator
    ? getEvaluatorIdentifier(drawerEvaluator)
    : "";
  const drawerEvaluatorAlreadySelected = drawerIdentifier
    ? selectedEvaluatorNames.includes(drawerIdentifier)
    : false;

  const drawerInitialConfig = useMemo(() => {
    if (!drawerIdentifier) {
      return undefined;
    }
    return selectedEvaluators.find(
      (item) => item.identifier === drawerIdentifier,
    )?.config;
  }, [drawerIdentifier, selectedEvaluators]);

  const handleConfirmEvaluator = useCallback(
    (config: Record<string, unknown>) => {
      if (!drawerEvaluator || !drawerIdentifier) {
        return;
      }
      onSaveEvaluatorConfig(drawerEvaluator, config);
      if (drawerEvaluator.type === "llm_judge") {
        setLlmJudgeIds((prev) => new Set(Array.from(prev).concat(drawerIdentifier)));
        if (!selectedProviderName) {
          handleCloseDrawer();
          setProviderDrawerOpen(true);
          return;
        }
      }
      handleCloseDrawer();
    },
    [
      drawerEvaluator,
      drawerIdentifier,
      handleCloseDrawer,
      onSaveEvaluatorConfig,
      selectedProviderName,
    ],
  );

  const handleRemoveEvaluator = useCallback(() => {
    if (!drawerEvaluator || !drawerIdentifier) {
      return;
    }
    if (selectedEvaluatorNames.includes(drawerIdentifier)) {
      onToggleEvaluator(drawerEvaluator);
    }
    if (drawerEvaluator.type === "llm_judge") {
      const isLastLLMJudge = llmJudgeIds.size === 1 && llmJudgeIds.has(drawerIdentifier);
      setLlmJudgeIds((prev) => {
        const next = new Set(Array.from(prev));
        next.delete(drawerIdentifier);
        return next;
      });
      if (isLastLLMJudge) {
        onLLMProviderChange(undefined);
      }
    }
    handleCloseDrawer();
  }, [
    drawerEvaluator,
    drawerIdentifier,
    handleCloseDrawer,
    llmJudgeIds,
    onLLMProviderChange,
    onToggleEvaluator,
    selectedEvaluatorNames,
  ]);

  return (
    <Form.Stack>
      <Form.Section>
        <Form.Header>
          <Stack spacing={0.5}>
            <Stack
              direction="row"
              alignItems="center"
              justifyContent="space-between"
              spacing={1}
            >
              Evaluators
              <Stack direction="row" alignItems="center" spacing={2}>
                {selectedProviderName ? (
                  <Button
                    variant="text"
                    size="small"
                    startIcon={<Check size={14} />}
                    onClick={() => setProviderDrawerOpen(true)}
                    sx={{ color: "success.main", whiteSpace: "nowrap" }}
                  >
                    LLM: {providerDisplayName}
                  </Button>
                ) : (
                  <Button
                    variant="text"
                    size="small"
                    startIcon={<Settings size={14} />}
                    onClick={() => setProviderDrawerOpen(true)}
                    sx={{ whiteSpace: "nowrap" }}
                  >
                    Configure LLM
                  </Button>
                )}
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
            </Stack>
            {llmProviderError && (
              <Typography variant="caption" color="error">
                {llmProviderError}
              </Typography>
            )}
          </Stack>
        </Form.Header>
        {selectedChipEvaluators.length > 0 && (
          <Form.Section>
            <Stack direction="row" spacing={2} flexWrap="wrap" alignItems="center">
              {selectedChipEvaluators.map((evaluator) => {
                const identifier = getEvaluatorIdentifier(evaluator);
                return (
                  <Box py={0.25} key={identifier}>
                    <Chip
                      label={evaluator.displayName}
                      onDelete={() => {
                        if (llmJudgeIds.has(identifier)) {
                          const isLastLLMJudge = llmJudgeIds.size === 1;
                          setLlmJudgeIds((prev) => {
                            const next = new Set(Array.from(prev));
                            next.delete(identifier);
                            return next;
                          });
                          if (isLastLLMJudge) {
                            onLLMProviderChange(undefined);
                          }
                        }
                        onToggleEvaluator({
                          id: identifier,
                          identifier,
                          displayName: evaluator.displayName,
                          description: "",
                          version: "",
                          provider: "",
                          level: "trace",
                          tags: [],
                          isBuiltin: true,
                          configSchema: [],
                        } as EvaluatorResponse);
                      }}
                      color="primary"
                    />
                  </Box>
                );
              })}
            </Stack>
          </Form.Section>
        )}
        {!orgId && (
          <Alert severity="warning" sx={{ mt: 2 }}>
            Unable to determine organization. Navigate from the project context
            to load evaluators.
          </Alert>
        )}
        {evaluatorsError ? (
          <Alert severity="error" sx={{ mt: 2 }}>
            {getErrorMessage(evaluatorsError) || "Failed to load evaluators"}
          </Alert>
        ) : null}
        {isLoading && (
          <Stack direction="row" gap={1} p={2}>
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
          </Stack>
        )}
        {!isLoading &&
          orgId &&
          !evaluatorsError &&
          evaluators.length === 0 &&
          !search.trim() && (
            <ListingTable.Container sx={{ my: 3 }}>
              <ListingTable.EmptyState
                illustration={<CircleIcon size={64} />}
                title="No evaluators yet"
                description="Connect evaluator providers or import custom evaluators to see them here."
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
            {evaluators.map((monitor) => {
              const identifier = getEvaluatorIdentifier(monitor);
              const isSelected = selectedEvaluators.some(
                (item) => item.identifier === identifier,
              );
              return (
                <Form.CardButton
                  key={monitor.id}
                  sx={{
                    width: "100%",
                    minWidth: 0,
                    justifyContent: "flex-start",
                    overflow: "hidden",
                  }}
                  selected={isSelected}
                  onClick={() => handleOpenDrawer(monitor)}
                >
                  <CardHeader
                    sx={{
                      overflow: "hidden",
                      minWidth: 0,
                      width: "100%",
                      "& .MuiCardHeader-content": {
                        overflow: "hidden",
                        minWidth: 0,
                      },
                    }}
                    title={
                      <Stack
                        direction="row"
                        spacing={1}
                        alignItems="center"
                        sx={{ minWidth: 0, overflow: "hidden" }}
                      >
                        <Stack
                          direction="column"
                          spacing={2}
                          sx={{ minWidth: 0, overflow: "hidden" }}
                        >
                          <Stack
                            direction="row"
                            spacing={2}
                            alignItems="center"
                            sx={{ minWidth: 0, overflow: "hidden" }}
                          >
                            <Avatar
                              sx={{
                                bgcolor: isSelected
                                  ? "primary.main"
                                  : "default",
                                color: isSelected
                                  ? "primary.contrastText"
                                  : "text.secondary",
                                width: 40,
                                height: 40,
                                flexShrink: 0,
                              }}
                            >
                              {isSelected ? (
                                <Check size={20} />
                              ) : (
                                <CircleIcon size={20} />
                              )}
                            </Avatar>
                            <Stack
                              direction="row"
                              flexGrow={1}
                              spacing={1}
                              alignItems="center"
                              sx={{ minWidth: 0, overflow: "hidden" }}
                            >
                              <Tooltip
                                title={monitor.displayName}
                                placement="top"
                              >
                                <Typography
                                  variant="h6"
                                  textOverflow="ellipsis"
                                  overflow="hidden"
                                  whiteSpace="nowrap"
                                  sx={{ flexShrink: 1, minWidth: 0 }}
                                >
                                  {monitor.displayName}
                                </Typography>
                              </Tooltip>
                              {monitor?.level && (
                                <Chip
                                  label={
                                    monitor.level.charAt(0).toUpperCase() +
                                    monitor.level.slice(1)
                                  }
                                  size="small"
                                  variant="outlined"
                                  color="primary"
                                  sx={{ flexShrink: 0 }}
                                />
                              )}
                            </Stack>
                          </Stack>
                          <Stack
                            direction="row"
                            spacing={1}
                            alignItems="center"
                          >
                            {(monitor.tags ?? []).slice(0, 4).map((tag) => (
                              <Chip
                                key={tag}
                                size="small"
                                label={tag}
                                variant="outlined"
                              />
                            ))}
                            {(monitor.tags ?? []).length > 4 && (
                              <Tooltip
                                title={(monitor.tags ?? []).join(", ")}
                                placement="top"
                              >
                                <Typography
                                  variant="caption"
                                  color="text.secondary"
                                >
                                  {`+${(monitor.tags ?? []).length - 4} more`}
                                </Typography>
                              </Tooltip>
                            )}
                          </Stack>
                        </Stack>
                      </Stack>
                    }
                  />
                  <CardContent>
                    <Stack spacing={1}>
                      <Typography variant="caption">
                        {monitor.description}
                      </Typography>
                    </Stack>
                  </CardContent>
                </Form.CardButton>
              );
            })}
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
        {error && (
          <Typography variant="caption" color="error" sx={{ mt: 1 }}>
            {error}
          </Typography>
        )}
      </Form.Section>
      <EvaluatorDetailsDrawer
        evaluator={drawerEvaluator}
        open={Boolean(drawerEvaluator)}
        onClose={handleCloseDrawer}
        isSelected={drawerEvaluatorAlreadySelected}
        initialConfig={drawerInitialConfig}
        onAdd={handleConfirmEvaluator}
        onRemove={handleRemoveEvaluator}
      />
      <MonitorLLMProviderDrawer
        open={providerDrawerOpen}
        onClose={() => setProviderDrawerOpen(false)}
        selectedProviderName={selectedProviderName}
        onProviderChange={handleProviderChange}
      />
    </Form.Stack>
  );
}

export default SelectPresetMonitors;

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
  Divider,
  Form,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { getErrorMessage } from "@agent-management-platform/shared-component";
import {
  Check,
  CircleIcon,
  Plus,
  Search as SearchIcon,
  Settings,
} from "@wso2/oxygen-ui-icons-react";
import { absoluteRouteMap, type EvaluatorResponse, type MonitorEvaluator, type MonitorLLMProviderRef } from "@agent-management-platform/types";
import {
  useListCatalogLLMProviders,
  useListEvaluators,
  useListLLMProviders,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import { generatePath, useParams } from "react-router-dom";
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

const PAGE_SIZE = 12;

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
  const [offset, setOffset] = useState(0);
  const [allEvaluators, setAllEvaluators] = useState<EvaluatorResponse[]>([]);
  const {
    data,
    isLoading,
    error: evaluatorsError,
  } = useListEvaluators(
    {
      orgName: orgId,
    },
    {
      limit: PAGE_SIZE,
      offset,
      search: debouncedSearch.trim() || undefined,
    },
  );

  // Accumulate evaluators across pages; offset reset to 0 (on search change) resets the list.
  // Deduplicate by id to guard against refetch-driven double-appends.
  useEffect(() => {
    if (!data?.evaluators) return;
    if (offset === 0) {
      setAllEvaluators(data.evaluators);
    } else {
      setAllEvaluators((prev) => {
        const existingIds = new Set(prev.map((e) => e.id));
        const newItems = data.evaluators.filter((e) => !existingIds.has(e.id));
        return newItems.length > 0 ? [...prev, ...newItems] : prev;
      });
    }
  }, [data, offset]);

  const totalItems = data?.total ?? allEvaluators.length;
  const hasMore = allEvaluators.length < totalItems;

  const selectedProviderName = llmProvider?.providerName;

  const { data: providersData } = useListLLMProviders({ orgName: orgId });
  const providerDisplayName = useMemo(
    () =>
      providersData?.providers.find((p) => p.id === selectedProviderName)
        ?.name ?? selectedProviderName,
    [providersData, selectedProviderName],
  );

  const { data: catalogProvidersData } = useListCatalogLLMProviders(
    { orgName: orgId },
    { limit: 100 },
  );
  const { data: llmTemplatesData } = useListLLMProviderTemplates({
    orgName: orgId,
  });

  const providerTemplateMap = useMemo(() => {
    const map = new Map<string, { displayName: string; logoUrl?: string }>();
    for (const t of llmTemplatesData?.templates ?? []) {
      map.set(t.name, { displayName: t.name, logoUrl: t.metadata?.logoUrl });
      map.set(t.id, { displayName: t.name, logoUrl: t.metadata?.logoUrl });
    }
    return map;
  }, [llmTemplatesData]);

  const providerLogoUrl = useMemo(() => {
    const entry = (catalogProvidersData?.entries ?? []).find(
      (e) => e.handle === selectedProviderName,
    );
    return entry ? providerTemplateMap.get(entry.template ?? "")?.logoUrl : undefined;
  }, [catalogProvidersData, selectedProviderName, providerTemplateMap]);

  const [llmJudgeIds, setLlmJudgeIds] = useState<Set<string>>(() => new Set());

  // Accumulate evaluator types across page loads so hasLLMJudge is correct in
  // edit mode even when a pre-selected LLM-judge is not on the current page.
  const [evaluatorTypeMap, setEvaluatorTypeMap] = useState<
    Map<string, string>
  >(() => new Map());

  useEffect(() => {
    if (allEvaluators.length === 0) return;
    setEvaluatorTypeMap((prev) => {
      const next = new Map(prev);
      for (const e of allEvaluators) {
        if (e.type !== undefined) {
          next.set(getEvaluatorIdentifier(e), e.type);
        }
      }
      return next;
    });
  }, [allEvaluators]);

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

  const llmJudgeIdsOnPage = useMemo(
    () =>
      new Set(
        allEvaluators
          .filter((e) => e.type === "llm_judge")
          .map(getEvaluatorIdentifier),
      ),
    [allEvaluators],
  );

  useEffect(() => {
    const judgesInSelection = new Set(
      selectedEvaluators
        .filter((e) => llmJudgeIdsOnPage.has(getEvaluatorIdentifier(e)))
        .map(getEvaluatorIdentifier),
    );
    setLlmJudgeIds((prev) => {
      const merged = new Set(Array.from(prev).concat(Array.from(judgesInSelection)));
      const selectedNames = new Set(selectedEvaluators.map(getEvaluatorIdentifier));
      Array.from(merged).forEach((id) => {
        if (!selectedNames.has(id)) merged.delete(id);
      });
      return merged;
    });
  }, [selectedEvaluators, llmJudgeIdsOnPage]);

  const hasLLMJudge = useMemo(
    () =>
      selectedEvaluatorNames.some(
        (id) => evaluatorTypeMap.get(id) === "llm_judge" || llmJudgeIdsOnPage.has(id),
      ) || llmJudgeIds.size > 0,
    [selectedEvaluatorNames, evaluatorTypeMap, llmJudgeIdsOnPage, llmJudgeIds],
  );

  useEffect(() => {
    onHasLLMJudgeChange(hasLLMJudge);
  }, [hasLLMJudge, onHasLLMJudgeChange]);

  const debouncedSetSearch = useMemo(
    () =>
      debounce((value: string) => {
        setDebouncedSearch(value);
        setOffset(0);
        setAllEvaluators([]);
      }, 300),
    [],
  );

  useEffect(
    () => () => {
      debouncedSetSearch.cancel();
    },
    [debouncedSetSearch],
  );

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
      (item) => getEvaluatorIdentifier(item) === drawerIdentifier,
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

  const createEvaluatorHref = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.evaluators.children.create.path,
        { orgId },
      )
    : undefined;

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
                  <Tooltip title="Change LLM provider" placement="top" arrow>
                    <Box
                      role="button"
                      tabIndex={0}
                      onClick={() => setProviderDrawerOpen(true)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          setProviderDrawerOpen(true);
                        }
                      }}
                      sx={{
                        display: "inline-flex",
                        alignItems: "center",
                        height: 24,
                        border: "1px solid",
                        borderColor: "primary.main",
                        borderRadius: "12px",
                        cursor: "pointer",
                        overflow: "hidden",
                        userSelect: "none",
                        "&:hover": { bgcolor: "action.hover" },
                      }}
                    >
                      <Box
                        sx={{
                          width: 28,
                          height: "100%",
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                          flexShrink: 0,
                          color: "primary.main",
                        }}
                      >
                        {providerLogoUrl ? (
                          <Box
                            component="img"
                            src={providerLogoUrl}
                            alt=""
                            width={16}
                            height={16}
                            sx={{ borderRadius: "50%", display: "block" }}
                          />
                        ) : (
                          <Check size={14} />
                        )}
                      </Box>
                      <Divider
                        orientation="vertical"
                        flexItem
                        sx={{ borderColor: "primary.main", opacity: 0.4 }}
                      />
                      <Typography
                        variant="caption"
                        color="primary"
                        sx={{ px: 1, fontWeight: 500, whiteSpace: "nowrap" }}
                      >
                        {providerDisplayName}
                      </Typography>
                    </Box>
                  </Tooltip>
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
                        const isJudge =
                          evaluatorTypeMap.get(identifier) === "llm_judge" ||
                          llmJudgeIds.has(identifier);
                        if (isJudge) {
                          const selectedJudgeCount = selectedEvaluatorNames.filter(
                            (id) =>
                              evaluatorTypeMap.get(id) === "llm_judge" ||
                              llmJudgeIds.has(id),
                          ).length;
                          const isLastLLMJudge = selectedJudgeCount === 1;
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
        {isLoading && allEvaluators.length === 0 && (
          <Stack direction="row" gap={1} p={2}>
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
          </Stack>
        )}
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
          {/* Create custom evaluator card — always first */}
          {!isLoading && orgId && createEvaluatorHref && (
            <Form.CardButton
              sx={{
                width: "100%",
                minWidth: 0,
                overflow: "hidden",
                border: "2px dashed",
                borderColor: "divider",
                transition: "border-color 0.2s",
                "&:hover": {
                  borderColor: "primary.main",
                  "& .create-evaluator-icon": {
                    bgcolor: "primary.main",
                    color: "primary.contrastText",
                  },
                },
              }}
              onClick={() => window.open(createEvaluatorHref, "_blank")}
            >
              <Box
                sx={{
                  width: "100%",
                  py: 3,
                  display: "flex",
                  flexDirection: "column",
                  alignItems: "center",
                  gap: 1.5,
                }}
              >
                <Avatar
                  className="create-evaluator-icon"
                  sx={{
                    bgcolor: "action.hover",
                    color: "text.secondary",
                    width: 52,
                    height: 52,
                    transition: "background-color 0.2s, color 0.2s",
                  }}
                >
                  <Plus size={28} />
                </Avatar>
                <Stack alignItems="center" spacing={0.5}>
                  <Typography variant="subtitle1" fontWeight={600}>
                    Create custom evaluator
                  </Typography>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    textAlign="center"
                    sx={{ px: 2 }}
                  >
                    Define your own evaluator with custom code or an LLM judge.
                  </Typography>
                </Stack>
              </Box>
            </Form.CardButton>
          )}

          {allEvaluators.map((monitor) => {
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
                              bgcolor: isSelected ? "primary.main" : "default",
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
                            <Tooltip title={monitor.displayName} placement="top">
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
                        <Stack direction="row" spacing={1} alignItems="center">
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
                              <Typography variant="caption" color="text.secondary">
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
                    <Typography variant="caption">{monitor.description}</Typography>
                  </Stack>
                </CardContent>
              </Form.CardButton>
            );
          })}
        </Box>

        {!isLoading &&
          orgId &&
          !evaluatorsError &&
          allEvaluators.length === 0 &&
          !search.trim() && (
            <ListingTable.Container sx={{ my: 3 }}>
              <ListingTable.EmptyState
                illustration={<CircleIcon size={64} />}
                title="No evaluators yet"
                description="Use the card above to create your first custom evaluator."
              />
            </ListingTable.Container>
          )}
        {allEvaluators.length === 0 && !isLoading && search.trim() && (
          <ListingTable.Container sx={{ my: 3 }}>
            <ListingTable.EmptyState
              illustration={<SearchIcon size={64} />}
              title="No evaluators match your search"
              description="Try a different keyword or clear the search filter."
            />
          </ListingTable.Container>
        )}

        {hasMore && (
          <Box display="flex" justifyContent="center" py={2}>
            <Button
              variant="outlined"
              size="small"
              onClick={() => setOffset((o) => o + PAGE_SIZE)}
              disabled={isLoading}
            >
              {isLoading ? "Loading..." : "Load more"}
            </Button>
          </Box>
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
        onClose={() => {
          setProviderDrawerOpen(false);
          setPendingEvaluator(null);
        }}
        selectedProviderName={selectedProviderName}
        onProviderChange={handleProviderChange}
      />
    </Form.Stack>
  );
}

export default SelectPresetMonitors;

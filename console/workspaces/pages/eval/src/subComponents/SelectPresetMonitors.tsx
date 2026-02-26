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
  CardContent,
  CardHeader,
  Chip,
  Form,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Check,
  CircleIcon,
  Search as SearchIcon,
} from "@wso2/oxygen-ui-icons-react";
import type {
  EvaluatorResponse,
  MonitorEvaluator,
  MonitorLLMProviderConfig,
} from "@agent-management-platform/types";
import {
  useListEvaluatorLLMProviders,
  useListEvaluators,
} from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";
import { useMemo, useState, useCallback } from "react";
import EvaluatorDetailsDrawer from "./EvaluatorDetailsDrawer";

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
  llmProviderConfigs: MonitorLLMProviderConfig[];
  onLLMProviderConfigsChange: (configs: MonitorLLMProviderConfig[]) => void;
  error?: string;
}

export function SelectPresetMonitors({
  selectedEvaluators,
  onToggleEvaluator,
  onSaveEvaluatorConfig,
  llmProviderConfigs,
  onLLMProviderConfigsChange,
  error,
}: SelectPresetMonitorsProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const {
    data,
    isPending,
    error: evaluatorsError,
  } = useListEvaluators({
    orgName: orgId,
  });
  const evaluators = useMemo(() => data?.evaluators ?? [], [data]);
  const { data: llmProvidersData } = useListEvaluatorLLMProviders({
    orgName: orgId,
  });

  const llmProviders = useMemo(
    () => llmProvidersData?.list ?? [],
    [llmProvidersData],
  );

  const [search, setSearch] = useState("");
  const [drawerEvaluator, setDrawerEvaluator] =
    useState<EvaluatorResponse | null>(null);

  const selectedEvaluatorNames = useMemo(
    () => selectedEvaluators.map((item) => getEvaluatorIdentifier(item)),
    [selectedEvaluators],
  );

  const filteredEvaluators = useMemo(() => {
    const term = search.trim().toLowerCase();
    if (!term) {
      return evaluators;
    }
    return evaluators.filter((evaluator) => {
      const haystack = [
        evaluator.displayName,
        evaluator.identifier,
        evaluator.description,
        ...(evaluator.tags ?? []),
      ]
        .filter(Boolean)
        .map((value) => value?.toLowerCase() ?? "");
      return haystack.some((value) => value.includes(term));
    });
  }, [evaluators, search]);

  const selectedFullEval = evaluators.filter((evaluator) =>
    selectedEvaluatorNames.includes(getEvaluatorIdentifier(evaluator)),
  );

  const handleOpenDrawer = useCallback((evaluator: EvaluatorResponse) => {
    setDrawerEvaluator(evaluator);
  }, []);

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
      handleCloseDrawer();
    },
    [
      drawerEvaluator,
      drawerIdentifier,
      handleCloseDrawer,
      onSaveEvaluatorConfig,
    ],
  );

  const handleRemoveEvaluator = useCallback(() => {
    if (!drawerEvaluator || !drawerIdentifier) {
      return;
    }
    if (selectedEvaluatorNames.includes(drawerIdentifier)) {
      onToggleEvaluator(drawerEvaluator);
    }
    handleCloseDrawer();
  }, [
    drawerEvaluator,
    drawerIdentifier,
    handleCloseDrawer,
    onToggleEvaluator,
    selectedEvaluatorNames,
  ]);

  return (
    <Form.Stack>
      <Form.Section>
        <Form.Header>
          <Stack
            direction="row"
            spacing={1}
            alignItems="start"
            justifyContent="space-between"
          >
            Available Evaluators and Metrics
            <SearchBar
              placeholder="Search evaluators"
              size="small"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              disabled={!evaluators.length}
            />
          </Stack>
        </Form.Header>
        <Form.Section>
          <Stack
            direction="row"
            spacing={2}
            flexWrap="wrap"
            alignItems="center"
          >
            {selectedFullEval.map((evaluator) => {
              return (
                <Box py={0.25} key={evaluator.id}>
                  <Chip
                    label={evaluator.displayName}
                    onDelete={() => onToggleEvaluator(evaluator)}
                    color="primary"
                  />
                </Box>
              );
            })}
            {selectedFullEval.length === 0 && (
              <Typography variant="body2" color="text.secondary">
                No evaluators selected yet. Click on the cards below to select.
              </Typography>
            )}
          </Stack>
        </Form.Section>
        {!orgId && (
          <Alert severity="warning" sx={{ mt: 2 }}>
            Unable to determine organization. Navigate from the project context
            to load evaluators.
          </Alert>
        )}
        {evaluatorsError && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {evaluatorsError instanceof Error
              ? evaluatorsError.message
              : "Failed to load evaluators"}
          </Alert>
        )}
        {isPending && (
          <Stack direction="row" gap={1} p={2}>
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
            <Skeleton variant="rounded" height={160} width="100%" />
          </Stack>
        )}
        {!isPending && orgId && !evaluatorsError && evaluators.length === 0 && (
          <ListingTable.Container sx={{ my: 3 }}>
            <ListingTable.EmptyState
              illustration={<CircleIcon size={64} />}
              title="No evaluators yet"
              description="Connect evaluator providers or import custom evaluators to see them here."
            />
          </ListingTable.Container>
        )}
        {evaluators.length > 0 && filteredEvaluators.length === 0 && (
          <ListingTable.Container sx={{ my: 3 }}>
            <ListingTable.EmptyState
              illustration={<SearchIcon size={64} />}
              title="No evaluators match your search"
              description="Try a different keyword or clear the search filter."
            />
          </ListingTable.Container>
        )}
        {filteredEvaluators.length > 0 && (
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
            {filteredEvaluators.map((monitor) => {
              const identifier = getEvaluatorIdentifier(monitor);
              const isSelected = selectedEvaluators.some(
                (item) => item.identifier === identifier,
              );
              return (
                <Form.CardButton
                  key={monitor.id}
                  sx={{ width: "100%", justifyContent: "flex-start" }}
                  selected={isSelected}
                  onClick={() => handleOpenDrawer(monitor)}
                >
                  <CardHeader
                    title={
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Stack direction="column" spacing={2}>
                          <Stack
                            direction="row"
                            spacing={2}
                            alignItems="center"
                          >
                            <Avatar
                              sx={{
                                bgcolor: isSelected
                                  ? "primary.main"
                                  : "default",
                                width: 40,
                                height: 40,
                              }}
                            >
                              {isSelected ? (
                                <Check size={20} />
                              ) : (
                                <CircleIcon size={20} />
                              )}
                            </Avatar>
                            <Typography
                              variant="h5"
                              textOverflow="ellipsis"
                              overflow="hidden"
                              whiteSpace="nowrap"
                              maxWidth="90%"
                            >
                              {monitor.displayName}
                            </Typography>
                          </Stack>
                          <Stack
                            direction="row"
                            spacing={1}
                            alignItems="center"
                          >
                            {(monitor.tags ?? []).slice(0, 2).map((tag) => (
                              <Chip
                                key={tag}
                                size="small"
                                label={tag}
                                variant="outlined"
                              />
                            ))}
                            {(monitor.tags ?? []).length > 2 && (
                              <Tooltip
                                title={(monitor.tags ?? []).join(", ")}
                                placement="top"
                              >
                                <Typography
                                  variant="caption"
                                  color="text.secondary"
                                >
                                  {`+${(monitor.tags ?? []).length - 2} more`}
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
        llmProviderConfigs={llmProviderConfigs}
        onLLMProviderConfigsChange={onLLMProviderConfigsChange}
        llmProviders={llmProviders}
        onAdd={handleConfirmEvaluator}
        onRemove={handleRemoveEvaluator}
      />
    </Form.Stack>
  );
}

export default SelectPresetMonitors;

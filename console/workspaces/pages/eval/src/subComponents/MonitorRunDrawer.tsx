/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { useState } from "react";
import {
  Alert,
  Box,
  Chip,
  LinearProgress,
  Stack,
  StatCard,
  Tab,
  Tabs,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { ChartBar, Logs, Timer, Users } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  LogsPanel,
} from "@agent-management-platform/views";
import { useMonitorRunLogs } from "@agent-management-platform/api-client";
import {
  type EvaluationLevel,
  type EvaluatorScoreSummary,
  type MonitorRunResponse,
  type MonitorRunStatus,
} from "@agent-management-platform/types";
import ScoreChip, { scoreColor } from "./ScoreChip";
import { LEVEL_CONFIG, levelChipSx } from "./levelConfig";

const RUN_STATUS_CHIP_COLOR_MAP: Record<
  MonitorRunStatus,
  "success" | "warning" | "default" | "error"
> = {
  success: "success",
  running: "warning",
  pending: "default",
  failed: "error",
};

export interface MonitorRunDrawerProps {
  run: MonitorRunResponse;
  orgName: string;
  projectName: string;
  agentName: string;
  monitorName: string;
  onClose: () => void;
  traceWindowLabel: string;
  durationLabel: string;
}

export function MonitorRunDrawer({
  run,
  orgName,
  projectName,
  agentName,
  monitorName,
  onClose,
  traceWindowLabel,
  durationLabel,
}: MonitorRunDrawerProps) {
  const { data, isLoading, error } = useMonitorRunLogs({
    orgName,
    projName: projectName,
    agentName,
    monitorName,
    runId: run.id ?? "",
  });

  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";
  const [activeTab, setActiveTab] = useState(0);

  const logs = [...(data?.logs ?? [])].reverse();
  const logsEmptyState = {
    title: "No logs yet",
    description: "Run logs will appear once this monitor produces output.",
    illustration: <Logs size={64} />,
  };
  const chipColor = RUN_STATUS_CHIP_COLOR_MAP[run.status] ?? "default";
  const evaluatorCount = run.evaluators?.length ?? 0;

  const scores = run.scores ?? [];
  const scoredEvaluators = scores.filter(
    (e) => e.aggregations?.["mean"] != null,
  );
  const avgScore =
    scoredEvaluators.length > 0
      ? scoredEvaluators.reduce(
          (acc, e) => acc + (e.aggregations["mean"] as number),
          0,
        ) / scoredEvaluators.length
      : null;
  const hasScores = run.status === "success" && scores.length > 0;

  const statCards = [
    {
      label: "Duration",
      value: durationLabel || "—",
      icon: <Timer size={24} />,
      iconColor: "primary" as const,
    },
    ...(hasScores && avgScore != null
      ? [
          {
            label: "Avg Score",
            value: `${(avgScore * 100).toFixed(1)}%`,
            icon: <ChartBar size={24} />,
            iconColor: "success" as const,
          },
        ]
      : []),
    {
      label: "Evaluators",
      value: evaluatorCount.toString(),
      icon: <Users size={24} />,
      iconColor: "secondary" as const,
    },
  ];

  return (
    <Stack direction="column" height="100%" maxWidth={900} width="100%">
      <DrawerHeader
        icon={<Logs size={24} />}
        title="Run Details"
        onClose={onClose}
      />
      <DrawerContent>
        <Stack spacing={2} height="calc(100vh - 96px)">
          <Stack spacing={0.5} alignItems="center" direction="row">
            <Typography variant="h6">{traceWindowLabel}&nbsp;</Typography>
            <Box>
              <Chip
                size="small"
                variant="outlined"
                label={run.status.toUpperCase()}
                color={chipColor}
              />
            </Box>
          </Stack>

          <Stack direction="row" spacing={1.5}>
            {statCards.map((card) => (
              <StatCard
                key={card.label}
                label={card.label}
                value={card.value}
                icon={card.icon}
                sx={{ height: 80, flex: 1, minWidth: 0 }}
                iconColor={card.iconColor}
              />
            ))}
          </Stack>
          {run.errorMessage && (
            <Alert severity="error">{run.errorMessage}</Alert>
          )}
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              flex: 1,
              minHeight: 0,
            }}
          >
            <Tabs
              value={activeTab}
              onChange={(_, v) => setActiveTab(v as number)}
              sx={{ borderBottom: 1, borderColor: "divider", mb: 1 }}
            >
              <Tab label="Logs" />
              {hasScores && <Tab label="Scores" />}
              <Tab label="Evaluator Configs" />
            </Tabs>

            {activeTab === 0 && (
              <LogsPanel
                logs={logs}
                isLoading={isLoading}
                error={error}
                showSearch={false}
                maxHeight="calc(100vh - 300px)"
                emptyState={logsEmptyState}
              />
            )}

            {hasScores && activeTab === 1 && (
              <Box sx={{ overflowY: "auto", maxHeight: "calc(100vh - 300px)" }}>
                <Stack spacing={1.5}>
                  {scores.map((ev: EvaluatorScoreSummary) => {
                    const mean = ev.aggregations?.["mean"] as
                      | number
                      | null
                      | undefined;
                    const levelCfg =
                      LEVEL_CONFIG[ev.level as EvaluationLevel] ??
                      LEVEL_CONFIG.trace;
                    return (
                      <Box
                        key={ev.evaluatorName}
                        sx={{
                          border: 1,
                          borderColor: "divider",
                          borderRadius: 2,
                          p: 1.5,
                        }}
                      >
                        <Stack
                          direction="row"
                          justifyContent="space-between"
                          alignItems="center"
                          mb={0.5}
                        >
                          <Stack
                            direction="row"
                            alignItems="center"
                            spacing={1}
                          >
                            <Typography variant="subtitle2">
                              {ev.evaluatorName}
                            </Typography>
                            <Chip
                              size="small"
                              label={levelCfg.label}
                              sx={levelChipSx(levelCfg, isDark)}
                            />
                          </Stack>
                          {mean != null ? (
                            <ScoreChip score={mean} variant="chip" />
                          ) : (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                            >
                              N/A
                            </Typography>
                          )}
                        </Stack>
                        {mean != null && (
                          <LinearProgress
                            variant="determinate"
                            value={mean * 100}
                            sx={{
                              height: 4,
                              borderRadius: 2,
                              mb: 0.5,
                              "& .MuiLinearProgress-bar": {
                                backgroundColor: scoreColor(mean),
                              },
                            }}
                          />
                        )}
                        <Stack direction="row" spacing={2}>
                          <Typography variant="caption" color="text.secondary">
                            Evaluated: {ev.count}
                          </Typography>
                          {ev.skippedCount > 0 && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                            >
                              Skipped: {ev.skippedCount}
                            </Typography>
                          )}
                        </Stack>
                      </Box>
                    );
                  })}
                </Stack>
              </Box>
            )}

            {activeTab === (hasScores ? 2 : 1) && (
              <Box sx={{ overflowY: "auto", maxHeight: "calc(100vh - 300px)" }}>
                {(run.evaluators ?? []).length === 0 ? (
                  <Stack
                    alignItems="center"
                    justifyContent="center"
                    py={6}
                    gap={1}
                  >
                    <Typography variant="body2" fontWeight={500}>
                      No evaluators configured
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      This run has no evaluator configuration data.
                    </Typography>
                  </Stack>
                ) : (
                  <Stack spacing={2}>
                    {(run.evaluators ?? []).map((ev) => (
                      <Box
                        key={ev.identifier}
                        sx={{
                          border: 1,
                          borderColor: "divider",
                          borderRadius: 2,
                          p: 2,
                        }}
                      >
                        <Stack
                          direction="row"
                          alignItems="center"
                          spacing={1}
                          mb={1}
                        >
                          <Typography variant="subtitle2">
                            {ev.displayName ?? ev.identifier}
                          </Typography>
                        </Stack>
                        {ev.config && Object.keys(ev.config).length > 0 ? (
                          <Stack spacing={0.75}>
                            {Object.entries(ev.config).map(([k, v]) => (
                              <Stack
                                key={k}
                                direction="row"
                                spacing={1}
                                alignItems="flex-start"
                              >
                                <Typography
                                  variant="caption"
                                  color="text.secondary"
                                  sx={{ minWidth: 120 }}
                                >
                                  {k}
                                </Typography>
                                <Typography
                                  variant="caption"
                                  sx={{
                                    fontFamily: "monospace",
                                    wordBreak: "break-all",
                                  }}
                                >
                                  {typeof v === "object"
                                    ? JSON.stringify(v)
                                    : String(v)}
                                </Typography>
                              </Stack>
                            ))}
                          </Stack>
                        ) : (
                          <Typography variant="caption" color="text.secondary">
                            No configuration parameters.
                          </Typography>
                        )}
                      </Box>
                    ))}
                  </Stack>
                )}
              </Box>
            )}
          </Box>
        </Stack>
      </DrawerContent>
    </Stack>
  );
}

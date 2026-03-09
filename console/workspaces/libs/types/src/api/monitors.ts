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

import { type AgentPathParams } from "./common";

export type EvaluationLevel = "trace" | "agent" | "llm";
export type MonitorScoreGranularity = "hour" | "day" | "week";

export type MonitorType = "future" | "past";
export type MonitorStatus = "Active" | "Suspended" | "Failed" | "Unknown";
export type MonitorRunStatus = "pending" | "running" | "success" | "failed";

export interface MonitorEvaluator {
  identifier: string;
  displayName: string;
  config?: Record<string, unknown>;
}

export interface MonitorLLMProviderConfig {
  providerName: string;
  envVar: string;
  value?: string;
}

export interface MonitorRunResponse {
  id: string;
  monitorName?: string;
  evaluators: MonitorEvaluator[];
  traceStart: string;
  traceEnd: string;
  startedAt?: string;
  completedAt?: string;
  status: MonitorRunStatus;
  errorMessage?: string;
  scores?: EvaluatorScoreSummary[];
}

export interface MonitorResponse {
  id: string;
  name: string;
  displayName: string;
  description?: string;
  type: MonitorType;
  orgName: string;
  projectName: string;
  agentName: string;
  environmentName: string;
  evaluators: MonitorEvaluator[];
  llmProviderConfigs?: MonitorLLMProviderConfig[];
  intervalMinutes?: number;
  nextRunTime?: string;
  traceStart?: string;
  traceEnd?: string;
  samplingRate: number;
  status: MonitorStatus;
  latestRun?: MonitorRunResponse;
  createdAt: string;
}

export interface MonitorListResponse {
  monitors: MonitorResponse[];
  total: number;
}

export interface MonitorRunListResponse {
  runs: MonitorRunResponse[];
  total: number;
}

export interface MonitorRunScoresResponse {
  runId: string;
  monitorName: string;
  evaluators: EvaluatorScoreSummary[];
}

export interface CreateMonitorRequest {
  name: string;
  displayName: string;
  description?: string;
  environmentName: string;
  evaluators: MonitorEvaluator[];
  llmProviderConfigs?: MonitorLLMProviderConfig[];
  type: MonitorType;
  intervalMinutes?: number;
  traceStart?: string;
  traceEnd?: string;
  samplingRate?: number;
}

export interface UpdateMonitorRequest {
  displayName?: string;
  evaluators?: MonitorEvaluator[];
  llmProviderConfigs?: MonitorLLMProviderConfig[];
  intervalMinutes?: number;
  samplingRate?: number;
}

export type ListMonitorsPathParams = AgentPathParams;
export type CreateMonitorPathParams = AgentPathParams;

export interface MonitorPathParams extends AgentPathParams {
  monitorName: string | undefined;
}

export type GetMonitorPathParams = MonitorPathParams;
export type UpdateMonitorPathParams = MonitorPathParams;
export type DeleteMonitorPathParams = MonitorPathParams;
export type StopMonitorPathParams = MonitorPathParams;
export type StartMonitorPathParams = MonitorPathParams;
export type ListMonitorRunsPathParams = MonitorPathParams;

export interface ListMonitorRunsQueryParams {
  limit?: number;
  offset?: number;
  includeScores?: boolean;
}
export type MonitorScoresPathParams = MonitorPathParams;
export type MonitorScoresTimeSeriesPathParams = MonitorPathParams;

export interface MonitorRunPathParams extends MonitorPathParams {
  runId: string | undefined;
}

export type RerunMonitorPathParams = MonitorRunPathParams;
export type MonitorRunLogsPathParams = MonitorRunPathParams;

export interface MonitorScoresQueryParams {
  startTime?: string;
  endTime?: string;
  evaluator?: string;
  level?: EvaluationLevel;
}

export interface MonitorScoresTimeSeriesQueryParams {
  startTime?: string;
  endTime?: string;
  evaluators: string[];
}

export interface TimeRange {
  start: string;
  end: string;
}

export interface EvaluatorScoreSummary {
  evaluatorName: string;
  level: EvaluationLevel;
  count: number;
  skippedCount: number;
  aggregations: Record<string, unknown>;
}

export interface MonitorScoresResponse {
  monitorName: string;
  timeRange: TimeRange;
  evaluators: EvaluatorScoreSummary[];
}

export interface TimeSeriesPoint {
  timestamp: string;
  count: number;
  skippedCount: number;
  aggregations: Record<string, unknown>;
}

export interface TimeSeriesResponse {
  monitorName: string;
  evaluatorName: string;
  granularity: MonitorScoreGranularity;
  points: TimeSeriesPoint[];
}

export interface BatchTimeSeriesEvaluatorSeries {
  evaluatorName: string;
  points: TimeSeriesPoint[];
}

export interface BatchTimeSeriesResponse {
  monitorName: string;
  granularity: string;
  evaluators: BatchTimeSeriesEvaluatorSeries[];
}

export interface TraceEvaluatorScore {
  evaluatorName: string;
  score?: number | null;
  explanation?: string;
  skipReason?: string;
}

export interface EvaluatorScoreWithMonitor extends TraceEvaluatorScore {
  monitorName: string;
}

export interface TraceSpanGroup {
  spanId: string;
  spanLabel?: string;
  evaluators: TraceEvaluatorScore[];
}

export interface TraceMonitorGroup {
  monitorName: string;
  evaluators: TraceEvaluatorScore[];
  spans: TraceSpanGroup[];
}

export interface TraceScoresResponse {
  traceId: string;
  monitors: TraceMonitorGroup[];
}

export interface TraceScoreSummary {
  traceId: string;
  score?: number | null;
  totalCount: number;
  skippedCount: number;
}

export interface AgentTraceScoresResponse {
  traces: TraceScoreSummary[];
  totalCount: number;
}

export interface AgentTraceScoresParams extends AgentPathParams {
  startTime?: string;
  endTime?: string;
  limit?: number;
  offset?: number;
}

export interface TraceScoresPathParams extends AgentPathParams {
  traceId: string | undefined;
}

export interface LabelEvaluatorSummary {
  evaluatorName: string;
  mean: number;
  count: number;
  skippedCount: number;
}

export interface ScoreLabelGroup {
  label: string;
  evaluators: LabelEvaluatorSummary[];
}

export interface GroupedScoresResponse {
  monitorName: string;
  level: EvaluationLevel;
  timeRange: TimeRange;
  groups: ScoreLabelGroup[];
}

export interface GroupedScoresQueryParams {
  startTime?: string;
  endTime?: string;
  level: EvaluationLevel;
}

export type GroupedScoresPathParams = MonitorPathParams;

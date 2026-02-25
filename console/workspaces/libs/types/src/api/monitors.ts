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

export type EvaluationLevel = "trace" | "agent" | "span";
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
  evaluatorIdentifier: string;
  llmProviderId: string;
  model?: string;
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
}

export interface MonitorResponse {
  id: string;
  name: string;
  displayName: string;
  type: MonitorType;
  orgName: string;
  projectName: string;
  agentName: string;
  agentId?: string;
  environmentName: string;
  environmentId?: string;
  evaluators: MonitorEvaluator[];
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

export interface CreateMonitorRequest {
  name: string;
  displayName: string;
  projectName: string;
  agentName: string;
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
export type MonitorScoresPathParams = MonitorPathParams;
export type MonitorScoresTimeSeriesPathParams = MonitorPathParams;

export interface MonitorRunPathParams extends MonitorPathParams {
  runId: string | undefined;
}

export type RerunMonitorPathParams = MonitorRunPathParams;
export type MonitorRunLogsPathParams = MonitorRunPathParams;

export interface MonitorScoresQueryParams {
  startTime: string;
  endTime: string;
  evaluator?: string;
  level?: EvaluationLevel;
}

export interface MonitorScoresTimeSeriesQueryParams {
  startTime: string;
  endTime: string;
  evaluator: string;
  granularity?: MonitorScoreGranularity;
}

export interface TimeRange {
  start: string;
  end: string;
}

export interface EvaluatorScoreSummary {
  evaluatorName: string;
  level: EvaluationLevel;
  count: number;
  errorCount: number;
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
  errorCount: number;
  aggregations: Record<string, unknown>;
}

export interface TimeSeriesResponse {
  monitorName: string;
  evaluatorName: string;
  granularity: MonitorScoreGranularity;
  points: TimeSeriesPoint[];
}

export interface ScoreItem {
  spanId?: string | null;
  score?: number | null;
  explanation?: string | null;
  metadata?: Record<string, unknown>;
  error?: string | null;
}

export interface EvaluatorTraceGroup {
  evaluatorName: string;
  level: EvaluationLevel;
  scores: ScoreItem[];
}

export interface MonitorTraceGroup {
  monitorName: string;
  monitorId: string;
  runId: string;
  evaluators: EvaluatorTraceGroup[];
}

export interface TraceScoresResponse {
  traceId: string;
  monitors: MonitorTraceGroup[];
}

export interface TraceScoresPathParams extends AgentPathParams {
  traceId: string | undefined;
}

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

import { type OrgPathParams } from "./common";

export type MonitorType = "future" | "past";
export type MonitorStatus = "Active" | "Suspended" | "Failed" | "Unknown";
export type MonitorRunStatus = "pending" | "running" | "success" | "failed";

export interface MonitorEvaluator {
  name: string;
  config?: Record<string, unknown>;
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
  type: MonitorType;
  intervalMinutes?: number;
  traceStart?: string;
  traceEnd?: string;
  samplingRate?: number;
}

export interface UpdateMonitorRequest {
  displayName?: string;
  evaluators?: MonitorEvaluator[];
  intervalMinutes?: number;
  samplingRate?: number;
}

export type ListMonitorsPathParams = OrgPathParams;
export type CreateMonitorPathParams = OrgPathParams;

export interface MonitorPathParams extends OrgPathParams {
  monitorName: string | undefined;
}

export type GetMonitorPathParams = MonitorPathParams;
export type UpdateMonitorPathParams = MonitorPathParams;
export type DeleteMonitorPathParams = MonitorPathParams;
export type StopMonitorPathParams = MonitorPathParams;
export type StartMonitorPathParams = MonitorPathParams;
export type ListMonitorRunsPathParams = MonitorPathParams;

export interface MonitorRunPathParams extends MonitorPathParams {
  runId: string | undefined;
}

export type RerunMonitorPathParams = MonitorRunPathParams;
export type MonitorRunLogsPathParams = MonitorRunPathParams;

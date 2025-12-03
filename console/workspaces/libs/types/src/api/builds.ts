/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { type AgentPathParams, type BuildPathParams, type ListQuery, type PaginationMeta } from './common';

// Requests
export interface BuildAgentQuery {
  commitId?: string;
}

// Responses
export type BuildStatus = 'BuildInProgress' | 'BuildTriggered' | 'Completed' | 'BuildFailed';

export interface BuildResponse {
  buildId?: string;
  buildName: string;
  projectName: string;
  agentName: string;
  commitId: string;
  startedAt: string; // ISO date-time
  endedAt?: string; // ISO date-time
  imageId?: string;
  status?: BuildStatus;
  branch: string;
}

export interface BuildsListResponse extends PaginationMeta {
  builds: BuildResponse[];
}

export type LogLevel = 'INFO' | 'WARN' | 'ERROR' | 'DEBUG';

export interface BuildLogEntry {
  timestamp: string; // ISO date-time
  log: string;
  logLevel: LogLevel;
}

export type BuildLogsResponse = BuildLogEntry[];

export type BuildStepType = 'BuildInitiated' | 'BuildTriggered' | 'BuildCompleted' | 'WorkloadUpdated';
export type BuildStepStatus = 'True' | 'False' | 'Unknown';

export interface BuildStep {
  type: string; // Using string to be flexible with backend step types
  status: string; // Using string to be flexible with backend status values
  message: string;
  at: string; // ISO date-time
}

export interface BuildDetailsResponse extends BuildResponse {
  percent?: number; // 0-100
  steps?: BuildStep[];
  durationSeconds?: number;
}

// Path/Query helpers
export type BuildAgentPathParams = AgentPathParams;
export type GetAgentBuildsPathParams = AgentPathParams;
export type GetBuildPathParams = BuildPathParams;
export type GetBuildLogsPathParams = BuildPathParams;

export type GetAgentBuildsQuery = ListQuery;



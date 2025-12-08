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



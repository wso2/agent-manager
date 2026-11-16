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

import { httpGETObserver } from "../../shared/utils";
import type { LogsResponse } from "@agent-management-platform/types";

export interface ObserverRuntimeLogsParams {
  organization: string;
  project: string;
  agent: string;
  environment: string;
  startTime: string;
  endTime: string;
  searchPhrase?: string;
  logLevels?: string[];
  limit?: number;
  sortOrder?: "asc" | "desc";
}

function assertRequired(value: string, field: string): void {
  if (!value?.trim()) throw new Error(`Missing required parameters: ${field}`);
}

export async function getAgentRuntimeLogs(
  params: ObserverRuntimeLogsParams,
  getToken?: () => Promise<string>,
): Promise<LogsResponse> {
  const { organization, project, agent, environment, startTime, endTime } = params;
  assertRequired(organization, "organization");
  assertRequired(project, "project");
  assertRequired(agent, "agent");
  assertRequired(environment, "environment");
  assertRequired(startTime, "startTime");
  assertRequired(endTime, "endTime");

  const token = getToken ? await getToken() : undefined;
  const searchParams: Record<string, string> = {
    organization, project, agent, environment, startTime, endTime,
  };
  if (params.searchPhrase) searchParams.searchPhrase = params.searchPhrase;
  if (params.logLevels?.length) searchParams.logLevels = params.logLevels.join(",");
  if (params.limit !== undefined) searchParams.limit = params.limit.toString();
  if (params.sortOrder) searchParams.sortOrder = params.sortOrder;

  const res = await httpGETObserver("/api/v1/logs", { searchParams, token });
  return res.json();
}

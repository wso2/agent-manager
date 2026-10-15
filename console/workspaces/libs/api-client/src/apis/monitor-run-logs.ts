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

import { httpGETObserver } from "../utils";
import type { LogsResponse } from "@agent-management-platform/types";

export interface ObserverMonitorRunLogsParams {
  organization: string;
  runName: string;
  startTime?: string;
  endTime?: string;
  limit?: number;
}

function assertRequired(value: string, field: string): void {
  if (!value?.trim()) throw new Error(`Missing required parameters: ${field}`);
}

export async function getMonitorRunLogs(
  params: ObserverMonitorRunLogsParams,
  getToken?: () => Promise<string>,
): Promise<LogsResponse> {
  const { organization, runName } = params;
  assertRequired(organization, "organization");
  assertRequired(runName, "runName");

  const token = getToken ? await getToken() : undefined;
  const searchParams: Record<string, string> = { organization, runName };
  // The observer rejects a half-specified range, so send both bounds or neither.
  if (params.startTime && params.endTime) {
    searchParams.startTime = params.startTime;
    searchParams.endTime = params.endTime;
  }
  if (params.limit !== undefined) searchParams.limit = params.limit.toString();

  const res = await httpGETObserver("/api/v1/monitor-run-logs", { searchParams, token });
  return res.json();
}

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

import {
  TraceDetailsResponse,
  TraceListResponse,
  TraceExportResponse,
} from "@agent-management-platform/types";
import { httpGETObserver } from "../utils";

// Internal params for direct trace-observer calls (use UUIDs, not names)
export interface TraceObserverListParams {
  componentUid: string;
  environmentUid: string;
  startTime?: string;
  endTime?: string;
  limit?: number;
  offset?: number;
  sortOrder?: 'asc' | 'desc';
}

export interface TraceObserverGetParams {
  traceId: string;
  componentUid: string;
  environmentUid: string;
}

export async function getTrace(
  params: TraceObserverGetParams,
  getToken?: () => Promise<string>
): Promise<TraceDetailsResponse> {
  const { traceId, componentUid, environmentUid } = params;

  const missingParams: string[] = [];
  if (!traceId) missingParams.push("traceId");
  if (!componentUid) missingParams.push("componentUid");
  if (!environmentUid) missingParams.push("environmentUid");

  if (missingParams.length > 0) {
    throw new Error(`Missing required parameters: ${missingParams.join(", ")}`);
  }

  const token = getToken ? await getToken() : undefined;

  const res = await httpGETObserver("/api/v1/trace", {
    searchParams: { traceId, componentUid, environmentUid },
    token,
  });

  return res.json();
}

export async function getTraceList(
  params: TraceObserverListParams,
  getToken?: () => Promise<string>
): Promise<TraceListResponse> {
  const { componentUid, environmentUid, startTime, endTime, limit, offset, sortOrder } = params;

  const missingParams: string[] = [];
  if (!componentUid) missingParams.push("componentUid");
  if (!environmentUid) missingParams.push("environmentUid");

  if (missingParams.length > 0) {
    throw new Error(`Missing required parameters: ${missingParams.join(", ")}`);
  }

  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = { componentUid, environmentUid };
  if (startTime) searchParams.startTime = startTime;
  if (endTime) searchParams.endTime = endTime;
  if (limit !== undefined) searchParams.limit = limit.toString();
  if (offset !== undefined) searchParams.offset = offset.toString();
  if (sortOrder) searchParams.sortOrder = sortOrder;

  const res = await httpGETObserver("/api/v1/traces", { searchParams, token });
  return res.json();
}

export async function exportTraces(
  params: TraceObserverListParams,
  getToken?: () => Promise<string>
): Promise<TraceExportResponse> {
  const { componentUid, environmentUid, startTime, endTime, limit, offset, sortOrder } = params;

  const missingParams: string[] = [];
  if (!componentUid) missingParams.push("componentUid");
  if (!environmentUid) missingParams.push("environmentUid");

  if (missingParams.length > 0) {
    throw new Error(`Missing required parameters: ${missingParams.join(", ")}`);
  }

  if ((startTime && !endTime) || (!startTime && endTime)) {
    throw new Error("startTime and endTime must be provided together");
  }

  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = { componentUid, environmentUid };
  if (startTime) searchParams.startTime = startTime;
  if (endTime) searchParams.endTime = endTime;
  if (limit !== undefined) searchParams.limit = limit.toString();
  if (offset !== undefined) searchParams.offset = offset.toString();
  if (sortOrder) searchParams.sortOrder = sortOrder;

  const res = await httpGETObserver("/api/v1/traces/export", { searchParams, token });
  return res.json();
}

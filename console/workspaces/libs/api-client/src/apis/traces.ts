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

import type {
  TraceListResponse,
  TraceExportResponse,
  TraceFilters,
  Span,
  TraceSpanSummaryListResponse,
} from "@agent-management-platform/types";
import { httpGETObserver } from "../utils";

// Params for direct observer service calls.
export interface ObserverTraceListParams {
  organization: string;
  project: string;
  component: string;
  environment: string;
  startTime: string;
  endTime: string;
  limit?: number;
  sortOrder?: 'asc' | 'desc';
  filters?: TraceFilters;
  /** Fill models on every trace; costs the server one extra upstream call per trace. */
  includeModels?: boolean;
  /** nextCursor from the previous page of the same window, sort order and filters. */
  cursor?: string;
}

/** Returns only the set filter fields, in a fixed order. */
export function normalizeTraceFilters(filters?: TraceFilters): TraceFilters {
  const out: TraceFilters = {};
  if (!filters) return out;
  if (filters.status) out.status = filters.status;
  if (filters.minDurationMs !== undefined) out.minDurationMs = filters.minDurationMs;
  if (filters.minTokens !== undefined) out.minTokens = filters.minTokens;
  if (filters.minSpanCount !== undefined) out.minSpanCount = filters.minSpanCount;
  if (filters.model) out.model = filters.model;
  if (filters.conversationId) out.conversationId = filters.conversationId;
  return out;
}

/** Query params for the set filter fields. */
export function traceFilterSearchParams(filters?: TraceFilters): Record<string, string> {
  const params: Record<string, string> = {};
  for (const [key, value] of Object.entries(normalizeTraceFilters(filters))) {
    params[key] = String(value);
  }
  return params;
}

export interface ObserverTraceSpanListParams {
  traceId: string;
  organization: string;
  project?: string;
  component?: string;
  environment?: string;
  startTime: string;
  endTime: string;
  limit?: number;
  sortOrder?: "asc" | "desc";
}

export interface ObserverTraceSpanDetailParams {
  traceId: string;
  spanId: string;
}

function assertRequired(value: string, field: string): void {
  if (!value?.trim()) throw new Error(`Missing required parameters: ${field}`);
}

export async function getTraceList(
  params: ObserverTraceListParams,
  getToken?: () => Promise<string>
): Promise<TraceListResponse> {
  const {
    organization,
    project,
    component,
    environment,
    startTime,
    endTime,
    limit,
    sortOrder,
    filters,
    includeModels,
    cursor,
  } = params;
  assertRequired(organization, "organization");
  assertRequired(project, "project");
  assertRequired(component, "component");
  assertRequired(environment, "environment");
  assertRequired(startTime, "startTime");
  assertRequired(endTime, "endTime");

  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    organization,
    project,
    agent: component,
    environment,
    startTime,
    endTime,
  };
  if (limit !== undefined) searchParams.limit = limit.toString();
  if (sortOrder) searchParams.sortOrder = sortOrder;
  Object.assign(searchParams, traceFilterSearchParams(filters));
  if (includeModels === true) searchParams.includeModels = "true";
  if (cursor) searchParams.cursor = cursor;

  const res = await httpGETObserver("/api/v1/traces", { searchParams, token });
  return res.json();
}

export async function exportTraces(
  params: ObserverTraceListParams,
  getToken?: () => Promise<string>
): Promise<TraceExportResponse> {
  const {
    organization,
    project,
    component,
    environment,
    startTime,
    endTime,
    limit,
    sortOrder,
  } = params;
  assertRequired(organization, "organization");
  assertRequired(project, "project");
  assertRequired(component, "component");
  assertRequired(environment, "environment");
  assertRequired(startTime, "startTime");
  assertRequired(endTime, "endTime");

  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    organization,
    project,
    agent: component,
    environment,
    startTime,
    endTime,
  };
  if (limit !== undefined) searchParams.limit = limit.toString();
  if (sortOrder) searchParams.sortOrder = sortOrder;

  const res = await httpGETObserver("/api/v1/traces/export", { searchParams, token });
  return res.json();
}

export async function listTraceSpans(
  params: ObserverTraceSpanListParams,
  getToken?: () => Promise<string>,
): Promise<TraceSpanSummaryListResponse> {
  const {
    traceId,
    organization,
    project,
    component,
    environment,
    startTime,
    endTime,
    limit,
    sortOrder,
  } = params;

  assertRequired(traceId, "traceId");
  assertRequired(organization, "organization");
  assertRequired(startTime, "startTime");
  assertRequired(endTime, "endTime");

  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    organization,
    startTime,
    endTime,
  };
  if (project) searchParams.project = project;
  if (component) searchParams.agent = component;
  if (environment) searchParams.environment = environment;
  if (limit !== undefined) searchParams.limit = limit.toString();
  if (sortOrder) searchParams.sortOrder = sortOrder;

  const res = await httpGETObserver(`/api/v1/traces/${encodeURIComponent(traceId)}/spans`, {
    searchParams,
    token,
  });
  return res.json();
}

export async function getSpanDetail(
  params: ObserverTraceSpanDetailParams,
  getToken?: () => Promise<string>,
): Promise<Span> {
  const { traceId, spanId } = params;
  assertRequired(traceId, "traceId");
  assertRequired(spanId, "spanId");

  const token = getToken ? await getToken() : undefined;
  const res = await httpGETObserver(
    `/api/v1/traces/${encodeURIComponent(traceId)}/spans/${encodeURIComponent(spanId)}`,
    { token },
  );
  return res.json();
}

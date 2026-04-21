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
  TraceListResponse,
  TraceExportResponse,
  Span,
  TraceSpanSummaryListResponse,
} from "@agent-management-platform/types";
import { httpGETObserver } from "../utils";

// Params for direct traces-observer-service calls.
export interface TraceObserverListParams {
  organization: string;
  project: string;
  component: string;
  environment: string;
  startTime: string;
  endTime: string;
  limit?: number;
  sortOrder?: 'asc' | 'desc';
}

export interface TraceObserverSpanListParams {
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

export interface TraceObserverSpanDetailParams {
  traceId: string;
  spanId: string;
}

function encodeRequired(value: string, field: string) {
  if (!value?.trim()) throw new Error(`Missing required parameters: ${field}`);
  return value;
}

export async function getTraceList(
  params: TraceObserverListParams,
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
  } = params;
  encodeRequired(organization, "organization");
  encodeRequired(project, "project");
  encodeRequired(component, "component");
  encodeRequired(environment, "environment");
  encodeRequired(startTime, "startTime");
  encodeRequired(endTime, "endTime");

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

  const res = await httpGETObserver("/api/v1/traces", { searchParams, token });
  return res.json();
}

export async function exportTraces(
  params: TraceObserverListParams,
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
  encodeRequired(organization, "organization");
  encodeRequired(project, "project");
  encodeRequired(component, "component");
  encodeRequired(environment, "environment");
  encodeRequired(startTime, "startTime");
  encodeRequired(endTime, "endTime");

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
  params: TraceObserverSpanListParams,
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

  encodeRequired(traceId, "traceId");
  encodeRequired(organization, "organization");
  encodeRequired(startTime, "startTime");
  encodeRequired(endTime, "endTime");

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
  params: TraceObserverSpanDetailParams,
  getToken?: () => Promise<string>,
): Promise<Span> {
  const { traceId, spanId } = params;
  encodeRequired(traceId, "traceId");
  encodeRequired(spanId, "spanId");

  const token = getToken ? await getToken() : undefined;
  const res = await httpGETObserver(
    `/api/v1/traces/${encodeURIComponent(traceId)}/spans/${encodeURIComponent(spanId)}`,
    { token },
  );
  return res.json();
}

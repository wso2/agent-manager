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
  Span,
} from "@agent-management-platform/types";
import { httpGETObserver } from "../utils";

// Params for direct traces-observer-service calls (use names + namespace)
export interface TraceObserverListParams {
  namespace: string;
  project: string;
  component: string;
  environment: string;
  startTime: string;
  endTime: string;
  limit?: number;
  sortOrder?: 'asc' | 'desc';
}

export interface TraceObserverGetParams {
  traceId: string;
  namespace: string;
  project: string;
  component: string;
  environment: string;
  startTime: string;
  endTime: string;
}

export interface TraceObserverSpanListParams {
  traceId: string;
  namespace: string;
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

type SpanSummary = {
  spanId: string;
};

type SpanSummaryListResponse = {
  spans: SpanSummary[];
  totalCount: number;
};

function encodeRequired(value: string, field: string) {
  if (!value?.trim()) throw new Error(`Missing required parameters: ${field}`);
  return value;
}

export async function getTrace(
  params: TraceObserverGetParams,
  getToken?: () => Promise<string>
): Promise<TraceDetailsResponse> {
  const { traceId, namespace, project, component, environment, startTime, endTime } = params;
  encodeRequired(traceId, "traceId");
  encodeRequired(namespace, "namespace");
  encodeRequired(project, "project");
  encodeRequired(component, "component");
  encodeRequired(environment, "environment");
  encodeRequired(startTime, "startTime");
  encodeRequired(endTime, "endTime");

  const token = getToken ? await getToken() : undefined;

  // The updated API does not expose a single "trace details" endpoint.
  // To preserve the existing api-client contract expected by the console UI,
  // we first list spans for the trace, then fetch span details for each span.
  const spanList = await listTraceSpans(
    {
      traceId,
      namespace,
      project,
      component,
      environment,
      startTime,
      endTime,
      limit: 1000,
      sortOrder: "asc",
    },
    token,
  );

  const spans = await Promise.all(
    (spanList.spans ?? []).map((s) =>
      getSpanDetail({ traceId, spanId: s.spanId }, getToken),
    ),
  );

  return { spans, totalCount: spanList.totalCount ?? spans.length };
}

export async function getTraceList(
  params: TraceObserverListParams,
  getToken?: () => Promise<string>
): Promise<TraceListResponse> {
  const { namespace, project, component, environment, startTime, endTime, limit, sortOrder } =
    params;
  encodeRequired(namespace, "namespace");
  encodeRequired(project, "project");
  encodeRequired(component, "component");
  encodeRequired(environment, "environment");
  encodeRequired(startTime, "startTime");
  encodeRequired(endTime, "endTime");

  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    namespace,
    project,
    component,
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
  const { namespace, project, component, environment, startTime, endTime, limit, sortOrder } =
    params;
  encodeRequired(namespace, "namespace");
  encodeRequired(project, "project");
  encodeRequired(component, "component");
  encodeRequired(environment, "environment");
  encodeRequired(startTime, "startTime");
  encodeRequired(endTime, "endTime");

  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    namespace,
    project,
    component,
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
  tokenOrGetToken?: string | (() => Promise<string>) | undefined,
): Promise<SpanSummaryListResponse> {
  const { traceId, namespace, project, component, environment, startTime, endTime, limit, sortOrder } =
    params;

  encodeRequired(traceId, "traceId");
  encodeRequired(namespace, "namespace");
  encodeRequired(startTime, "startTime");
  encodeRequired(endTime, "endTime");

  const token =
    typeof tokenOrGetToken === "function" ? await tokenOrGetToken() : tokenOrGetToken;

  const searchParams: Record<string, string> = { namespace, startTime, endTime };
  if (project) searchParams.project = project;
  if (component) searchParams.component = component;
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

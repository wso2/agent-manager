/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Columns the user can show or hide; the others are always shown.
export type TraceColumn = "conversation" | "model";

// Menu and URL order.
export const OPTIONAL_TRACE_COLUMNS: { key: TraceColumn; label: string }[] = [
  { key: "conversation", label: "Conversation" },
  { key: "model", label: "Model" },
];

export const DEFAULT_TRACE_COLUMNS: TraceColumn[] = ["conversation"];

const isTraceColumn = (value: string): value is TraceColumn =>
  OPTIONAL_TRACE_COLUMNS.some((c) => c.key === value);

// Reads the visible optional columns from `columns`; absent means the defaults, empty means none.
export function parseTraceColumns(searchParams: URLSearchParams): TraceColumn[] {
  const raw = searchParams.get("columns");
  if (raw === null) return DEFAULT_TRACE_COLUMNS;
  const picked = new Set(raw.split(",").map((s) => s.trim()).filter(isTraceColumn));
  return OPTIONAL_TRACE_COLUMNS.map((c) => c.key).filter((key) => picked.has(key));
}

// Returns a copy of searchParams with `columns` set, or removed when it matches the defaults.
export function withTraceColumns(
  searchParams: URLSearchParams,
  columns: TraceColumn[],
): URLSearchParams {
  const next = new URLSearchParams(searchParams);
  const ordered = OPTIONAL_TRACE_COLUMNS.map((c) => c.key).filter((key) => columns.includes(key));
  if (ordered.join(",") === DEFAULT_TRACE_COLUMNS.join(",")) next.delete("columns");
  else next.set("columns", ordered.join(","));
  return next;
}

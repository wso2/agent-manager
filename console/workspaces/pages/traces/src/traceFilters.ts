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

import type { TraceFilters } from "@agent-management-platform/types";

export type TraceFilterKey = keyof TraceFilters;
type NumericFilterKey = "minDurationMs" | "minTokens" | "minSpanCount";
type TextFilterKey = "model" | "conversationId" | "tool" | "mcpServer" | "evaluator";
type ScoreFilterKey = "minScore" | "maxScore";

// URL param names match the API query params; this is also the chip order.
export const TRACE_FILTER_KEYS: TraceFilterKey[] = [
  "status",
  "minDurationMs",
  "minTokens",
  "minSpanCount",
  "model",
  "conversationId",
  "tool",
  "toolError",
  "mcpServer",
  "evaluator",
  "minScore",
  "maxScore",
];

const NUMERIC_KEYS: NumericFilterKey[] = ["minDurationMs", "minTokens", "minSpanCount"];
const TEXT_KEYS: TextFilterKey[] = ["model", "conversationId", "tool", "mcpServer", "evaluator"];
const SCORE_KEYS: ScoreFilterKey[] = ["minScore", "maxScore"];

// The API rejects longer text filter values.
export const MAX_TEXT_FILTER_LENGTH = 256;

export const LATENCY_PRESETS_MS = [1000, 5000, 10000, 30000];
export const TOKEN_PRESETS = [1000, 5000, 10000, 50000];
export const STEP_PRESETS = [10, 20, 50];
export const SCORE_PRESETS = [0.25, 0.5, 0.75];

/** Formats ms as seconds when whole, e.g. 5s or 1500ms. */
export const formatLatency = (ms: number) =>
  ms % 1000 === 0 ? `${ms / 1000}s` : `${ms}ms`;

/** Formats whole thousands as k, e.g. 5k. */
export const formatTokens = (n: number) =>
  n >= 1000 && n % 1000 === 0 ? `${n / 1000}k` : `${n}`;

/** Formats a 0–1 score as a percentage, e.g. 50% or 33.33%. */
export const formatScore = (score: number) => `${Number((score * 100).toFixed(2))}%`;

/** Whether a score filter is set: a bound or an evaluator. */
export const hasScoreFilter = (filters: TraceFilters) =>
  filters.minScore !== undefined || filters.maxScore !== undefined || !!filters.evaluator;

/** Seconds as whole ms, e.g. "2.5" is 2500; undefined unless a plain non-negative decimal. */
export function parseLatencyInput(raw: string): number | undefined {
  const text = raw.trim();
  if (!/^(\d+\.?\d*|\.\d+)$/.test(text)) return undefined;
  const ms = Math.round(Number(text) * 1000);
  return Number.isSafeInteger(ms) ? ms : undefined;
}

/** A whole non-negative number, as the API takes for tokens and steps. */
export function parseCountInput(raw: string): number | undefined {
  const text = raw.trim();
  if (!/^\d+$/.test(text)) return undefined;
  const n = Number(text);
  return Number.isSafeInteger(n) ? n : undefined;
}

/** A percentage in [0, 100] as a 0–1 score rounded to 4 places, e.g. "33.5" is 0.335. */
export function parseScoreInput(raw: string): number | undefined {
  const text = raw.trim();
  if (!/^(\d+\.?\d*|\.\d+)$/.test(text) || Number(text) > 100) return undefined;
  return Number((Number(text) / 100).toFixed(4));
}

/** ms as seconds for a custom field, e.g. 2500 is "2.5". */
export const latencyInputValue = (ms: number) => String(ms / 1000);

/** A 0–1 score as a percentage for a custom field, e.g. 0.335 is "33.5". */
export const scoreInputValue = (score: number) => String(Number((score * 100).toFixed(2)));

// Reads filters from the URL, dropping anything the API would reject.
export function parseTraceFilters(searchParams: URLSearchParams): TraceFilters {
  const filters: TraceFilters = {};
  const status = searchParams.get("status");
  if (status === "error" || status === "ok") filters.status = status;
  for (const key of NUMERIC_KEYS) {
    const raw = searchParams.get(key) ?? "";
    // Non-negative integers only, as the API validates; parseInt would accept "5s".
    if (/^\d+$/.test(raw)) {
      const parsed = Number(raw);
      if (Number.isSafeInteger(parsed)) filters[key] = parsed;
    }
  }
  for (const key of TEXT_KEYS) {
    const raw = searchParams.get(key)?.trim();
    if (raw && raw.length <= MAX_TEXT_FILTER_LENGTH) filters[key] = raw;
  }
  if (searchParams.get("toolError") === "true") filters.toolError = true;
  for (const key of SCORE_KEYS) {
    const raw = searchParams.get(key) ?? "";
    // Plain decimals in [0, 1], as the API validates.
    if (/^(\d+\.?\d*|\.\d+)$/.test(raw) && Number(raw) <= 1) filters[key] = Number(raw);
  }
  // The API rejects minScore above maxScore; maxScore wins.
  if (
    filters.minScore !== undefined &&
    filters.maxScore !== undefined &&
    filters.minScore > filters.maxScore
  ) {
    delete filters.minScore;
  }
  return filters;
}

// Returns a copy of searchParams with the filter params replaced by filters.
export function withTraceFilters(
  searchParams: URLSearchParams,
  filters: TraceFilters,
): URLSearchParams {
  const next = new URLSearchParams(searchParams);
  for (const key of TRACE_FILTER_KEYS) {
    const value = filters[key];
    if (value === undefined || value === "" || value === false) next.delete(key);
    else next.set(key, String(value));
  }
  return next;
}

export interface TraceFilterChip {
  key: TraceFilterKey;
  // The whole chip, e.g. "Latency ≥ 5s"; used for the title and the × label.
  label: string;
  // The filter's name and value, shown apart: "Latency" and "≥ 5s".
  name: string;
  value: string;
  // Other filters the chip's × clears with key.
  alsoClears?: TraceFilterKey[];
}

// One human-readable chip per set filter, in TRACE_FILTER_KEYS order.
export function traceFilterChips(filters: TraceFilters): TraceFilterChip[] {
  const chips: TraceFilterChip[] = [];
  /** Adds a chip; label defaults to "name value". */
  const add = (key: TraceFilterKey, name: string, value: string, label = `${name} ${value}`) =>
    chips.push({ key, label, name, value });
  if (filters.status) {
    const status = filters.status === "error" ? "Error" : "OK";
    add("status", "Status", status, `Status: ${status}`);
  }
  if (filters.minDurationMs !== undefined) {
    add("minDurationMs", "Latency", `≥ ${formatLatency(filters.minDurationMs)}`);
  }
  if (filters.minTokens !== undefined) {
    add("minTokens", "Tokens", `≥ ${formatTokens(filters.minTokens)}`);
  }
  if (filters.minSpanCount !== undefined) {
    add("minSpanCount", "Steps", `≥ ${filters.minSpanCount}`);
  }
  if (filters.model) add("model", "Model", filters.model, `Model: ${filters.model}`);
  if (filters.conversationId) {
    add("conversationId", "Conversation", filters.conversationId,
      `Conversation: ${filters.conversationId}`);
  }
  // tool with toolError means that tool failed, so they share one chip.
  if (filters.tool && filters.toolError) {
    chips.push({
      key: "tool",
      label: `${filters.tool} failed`,
      name: "Tool",
      value: `${filters.tool} failed`,
      alsoClears: ["toolError"],
    });
  } else if (filters.tool) {
    add("tool", "Tool", filters.tool, `Tool: ${filters.tool}`);
  } else if (filters.toolError) {
    add("toolError", "Tool", "any failed", "Tool failed");
  }
  if (filters.mcpServer) {
    add("mcpServer", "MCP server", filters.mcpServer, `MCP server: ${filters.mcpServer}`);
  }
  if (filters.evaluator) {
    add("evaluator", "Evaluator", filters.evaluator, `Evaluator: ${filters.evaluator}`);
  }
  if (filters.minScore !== undefined) {
    add("minScore", "Score", `≥ ${formatScore(filters.minScore)}`);
  }
  if (filters.maxScore !== undefined) {
    add("maxScore", "Score", `≤ ${formatScore(filters.maxScore)}`);
  }
  return chips;
}

// Thresholds the drawer edits as pills with a custom value.
export type ThresholdKey = NumericFilterKey | ScoreFilterKey;

export interface ThresholdSpec {
  label: string;
  presets: number[];
  // Pill text for a value.
  format: (n: number) => string;
  // Reads a custom field; undefined when invalid.
  parse: (raw: string) => number | undefined;
  // Prefills the custom field from a value.
  inputValue: (n: number) => string;
  unit?: string;
  invalidHint: string;
}

const scoreSpec = (label: string): ThresholdSpec => ({
  label,
  presets: SCORE_PRESETS,
  format: formatScore,
  parse: parseScoreInput,
  inputValue: scoreInputValue,
  unit: "%",
  invalidHint: "Enter a percentage from 0 to 100",
});

export const THRESHOLDS: Record<ThresholdKey, ThresholdSpec> = {
  minDurationMs: {
    label: "Latency at least",
    presets: LATENCY_PRESETS_MS,
    format: formatLatency,
    parse: parseLatencyInput,
    inputValue: latencyInputValue,
    unit: "s",
    invalidHint: "Enter seconds, like 2.5",
  },
  minTokens: {
    label: "Tokens at least",
    presets: TOKEN_PRESETS,
    format: formatTokens,
    parse: parseCountInput,
    inputValue: String,
    invalidHint: "Enter a whole number",
  },
  minSpanCount: {
    label: "Steps at least",
    presets: STEP_PRESETS,
    format: String,
    parse: parseCountInput,
    inputValue: String,
    invalidHint: "Enter a whole number",
  },
  minScore: scoreSpec("Score at least"),
  maxScore: scoreSpec("Score at most"),
};

// The drawer's edits, applied together by Filter.
export interface TraceFilterDraft {
  // Every filter but status; text stays untrimmed until applied.
  filters: TraceFilters;
  // Raw text of each threshold set to Custom.
  custom: Partial<Record<ThresholdKey, string>>;
}

export const EMPTY_DRAFT: TraceFilterDraft = { filters: {}, custom: {} };

/** A draft holding the filters, without status, which the toggle owns. */
export function draftFrom(filters: TraceFilters): TraceFilterDraft {
  const rest = { ...filters };
  delete rest.status;
  return { filters: rest, custom: {} };
}

/** The draft without keys, in its filters and its custom values. */
export function draftWithout(draft: TraceFilterDraft, keys: TraceFilterKey[]): TraceFilterDraft {
  const filters = { ...draft.filters };
  const custom = { ...draft.custom };
  for (const key of keys) {
    delete filters[key];
    delete custom[key as ThresholdKey];
  }
  return { filters, custom };
}

/** The threshold's value in the draft: its parsed custom value when set to Custom. */
export function draftThreshold(draft: TraceFilterDraft, key: ThresholdKey): number | undefined {
  const raw = draft.custom[key];
  return raw === undefined ? draft.filters[key] : THRESHOLDS[key].parse(raw);
}

/** Whether a score bound of value would cross the draft's other bound. */
export function crossesOtherBound(
  draft: TraceFilterDraft,
  key: ThresholdKey,
  value: number,
): boolean {
  if (key === "minScore") {
    const max = draftThreshold(draft, "maxScore");
    return max !== undefined && value > max;
  }
  if (key === "maxScore") {
    const min = draftThreshold(draft, "minScore");
    return min !== undefined && value < min;
  }
  return false;
}

/** Why the threshold's custom value can't apply; undefined when it can or isn't custom. */
export function customError(draft: TraceFilterDraft, key: ThresholdKey): string | undefined {
  const raw = draft.custom[key];
  if (raw === undefined) return undefined;
  const value = THRESHOLDS[key].parse(raw);
  if (value === undefined) return THRESHOLDS[key].invalidHint;
  if (!crossesOtherBound(draft, key, value)) return undefined;
  return key === "minScore"
    ? `Must be at most ${formatScore(draftThreshold(draft, "maxScore")!)}`
    : `Must be at least ${formatScore(draftThreshold(draft, "minScore")!)}`;
}

/** Whether every custom value in the draft can apply. */
export const isDraftValid = (draft: TraceFilterDraft) =>
  (Object.keys(draft.custom) as ThresholdKey[]).every((key) => !customError(draft, key));

/** The filters Filter applies: the draft's, with custom values parsed, text trimmed, and status. */
export function appliedFilters(
  draft: TraceFilterDraft,
  status: TraceFilters["status"],
): TraceFilters {
  const next: TraceFilters = { ...draft.filters, status };
  for (const key of Object.keys(draft.custom) as ThresholdKey[]) {
    next[key] = draftThreshold(draft, key);
  }
  for (const key of TEXT_KEYS) next[key] = draft.filters[key]?.trim() || undefined;
  if (!next.toolError) delete next.toolError;
  for (const key of TRACE_FILTER_KEYS) if (next[key] === undefined) delete next[key];
  return next;
}

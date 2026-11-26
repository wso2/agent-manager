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

import type {
  CostByGroup,
  CostGroupBy,
  UnpricedRequests,
} from "@agent-management-platform/types";

export interface CostDimension {
  key: CostGroupBy;
  label: string;
  plural: string;
}

export const COST_DIMENSIONS: CostDimension[] = [
  { key: "organization", label: "Organization", plural: "Organization" },
  { key: "project", label: "Project", plural: "Projects" },
  { key: "agent", label: "Agent", plural: "Agents" },
  { key: "apiKey", label: "API key", plural: "API keys" },
  { key: "user", label: "User", plural: "Users" },
  { key: "llmProvider", label: "LLM provider", plural: "LLM providers" },
  { key: "model", label: "Model", plural: "Models" },
];

/** The dimensions a page can group by: a project's page has only those below a project. */
export function dimensionsFor(
  projectScoped: boolean,
  agentScoped = false,
): CostDimension[] {
  if (agentScoped)
    return COST_DIMENSIONS.filter(
      (d) =>
        d.key !== "organization" && d.key !== "project" && d.key !== "agent",
    );
  return projectScoped
    ? COST_DIMENSIONS.filter(
        (d) => d.key !== "organization" && d.key !== "project",
      )
    : COST_DIMENSIONS;
}

const usd = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  maximumFractionDigits: 0,
});
const usdCents = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

/** Shown for a cost above zero but under one cent, which two decimals can't show. */
const UNDER_ONE_CENT = "<$0.01";

/** Whole dollars for totals; cents below $100 so small spend doesn't read as $0. */
export function formatUsd(value: number): string {
  if (value > 0 && value < 0.01) return UNDER_ONE_CENT;
  return Math.abs(value) < 100 ? usdCents.format(value) : usd.format(value);
}

/** Dollars and cents, whatever the size: for figures read side by side. */
export function formatUsdCents(value: number): string {
  if (value > 0 && value < 0.01) return UNDER_ONE_CENT;
  return usdCents.format(value);
}

/** An API amount (decimal string) as a number; an unparsable one counts as 0. */
export function amount(value: string | undefined): number {
  const n = Number.parseFloat(value ?? "");
  return Number.isFinite(n) ? n : 0;
}

/** "2026-11" → "Nov", or "Nov 2026" with the year. */
export function monthLabel(month: string, withYear = false): string {
  const [y, m] = month.split("-").map(Number);
  if (!y || !m) return month;
  const date = new Date(Date.UTC(y, m - 1, 1));
  return date.toLocaleString("en-US", {
    month: withYear ? "long" : "short",
    year: withYear ? "numeric" : undefined,
    timeZone: "UTC",
  });
}

/** Shown for an entry whose requests carried no value (no user, API key or agent). */
export const UNATTRIBUTED = "Unattributed";

/** An entry's value for the picker; an agent is project/agent, as names are unique per project. */
export function groupKey(entry: CostByGroup, groupBy: CostGroupBy): string {
  if (groupBy === "agent") return `${entry.project ?? ""}/${entry.id ?? ""}`;
  return entry.id ?? "";
}

/**
 * Display names for grouped values, by kind: providers and projects by handle, agents by
 * project/agent. A value not listed keeps its handle.
 */
export interface CostNames {
  llmProvider?: Record<string, string>;
  project?: Record<string, string>;
  agent?: Record<string, string>;
}

/** What an entry is called on the page: its display name where one is known. */
export function groupLabel(
  key: string,
  groupBy: CostGroupBy,
  projectScoped: boolean,
  names: CostNames = {},
): string {
  if (groupBy === "agent") {
    const [project, agent] = key.split("/");
    if (!agent) return UNATTRIBUTED;
    const agentName = names.agent?.[key] ?? agent;
    if (projectScoped || !project) return agentName;
    return `${names.project?.[project] ?? project} / ${agentName}`;
  }
  if (!key) return UNATTRIBUTED;
  if (groupBy === "llmProvider") return names.llmProvider?.[key] ?? key;
  if (groupBy === "project") return names.project?.[key] ?? key;
  return key;
}

/** The kept months (YYYY-MM, oldest first): the report's, else the 3 UTC months up to now. */
export function keptMonths(entries: CostByGroup[], now = new Date()): string[] {
  if (entries[0]?.monthly.length) return entries[0].monthly.map((m) => m.month);
  return [2, 1, 0].map((back) => {
    const d = new Date(
      Date.UTC(now.getUTCFullYear(), now.getUTCMonth() - back, 1),
    );
    return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
  });
}

/** Total cost of the entries in each of the given months. */
export function monthlyTotals(
  entries: CostByGroup[],
  months: string[],
): number[] {
  return months.map((month) =>
    entries.reduce(
      (sum, e) => sum + amount(e.monthly.find((m) => m.month === month)?.cost),
      0,
    ),
  );
}

/** An entry's cost in each of the given months. */
export function entryMonthly(entry: CostByGroup, months: string[]): number[] {
  return monthlyTotals([entry], months);
}

/** Requests charged $0 over the entries, by reason. */
export function unpricedTotals(entries: CostByGroup[]): UnpricedRequests {
  return entries.reduce(
    (sum, e) => ({
      missingPrice: sum.missingPrice + e.unpricedRequests.missingPrice,
      usageUnavailable:
        sum.usageUnavailable + e.unpricedRequests.usageUnavailable,
    }),
    { missingPrice: 0, usageUnavailable: 0 },
  );
}

/** Change from one month to the next, as a whole percentage; undefined without a base. */
export function changePercent(
  previous: number,
  current: number,
): number | undefined {
  if (previous <= 0) return undefined;
  return Math.round(((current - previous) / previous) * 100);
}

/** A change as shown beside a month's cost: "+50% vs Sep", "-12% vs Oct". */
export function formatChange(change: number, against: string): string {
  return `${change > 0 ? "+" : ""}${change}% vs ${against}`;
}

/** Under a chart: the groups left out because they cost $0 (no price, or no usage). */
export function zeroCostNote(labels: string[]): string {
  const shown =
    labels.length > 3
      ? `${labels.slice(0, 3).join(", ")} and ${labels.length - 3} more`
      : labels.join(", ");
  return `Not shown (charged $0): ${shown}.`;
}

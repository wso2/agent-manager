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

import type { ListQuery, OrgPathParams, PaginationMeta } from "./common";

// Budgets and AI costs (agent-manager-service/modules/aigateway/api/openapi.yaml).

export const BUDGET_API_VERSION = "gateway.api-platform.wso2.com/v1";
/** Names the organization that owns a budget, by handle. */
export const BUDGET_ORG_ANNOTATION = "gateway.api-platform.wso2.com/org-id";
/** Names the project that owns a budget, by handle; absent for an organization budget. */
export const BUDGET_PROJECT_ANNOTATION = "gateway.api-platform.wso2.com/project-id";

export type BudgetPeriod = "day" | "week" | "month";
export type BudgetAction = "block" | "allow";
export type BudgetStatus = "active" | "inactive";
/** What a filter or split is about: one LLM provider, project, agent or API key, or a CEL
 * expression on the request (team, user, custom attribute). */
export type BudgetFilterType =
  | "llmProvider"
  | "project"
  | "agent"
  | "apiKey"
  | "attribute";
export type BudgetExemptionType = "project" | "agent" | "attribute";

/** A separate limit for each value of one dimension. Omitted: one shared limit. */
export interface BudgetSplit {
  type: BudgetFilterType;
  /** Attribute only: a CEL expression whose value tells the values apart. */
  expression?: string;
}

export interface BudgetLimit {
  /** Decimal string, at most two decimals. */
  amount: string;
  currency?: "USD";
  period: BudgetPeriod;
  action: BudgetAction;
  /** Block only: refuses once spend reaches this share of the amount (1-100). */
  blockAt?: number;
  splitBy?: BudgetSplit;
}

/** Which usage a budget counts; all of a budget's filters must match. */
export interface BudgetFilter {
  type: BudgetFilterType;
  /** LLM provider, project, agent or API key. */
  id?: string;
  /** Agent only: the agent's project (agent names are unique per project). */
  project?: string;
  /** Attribute only: a CEL expression (bool). */
  expression?: string;
}

export interface BudgetExemption {
  type: BudgetExemptionType;
  id?: string;
  /** Agent only: the agent's project. */
  project?: string;
  expression?: string;
}

export interface BudgetWarnings {
  thresholds: number[];
  /** Notification channel (event destination) names. */
  channels: string[];
}

export interface BudgetSpec {
  displayName?: string;
  description?: string;
  status?: BudgetStatus;
  limit: BudgetLimit;
  filters?: BudgetFilter[];
  exemptions?: BudgetExemption[];
  warnings?: BudgetWarnings;
}

export interface BudgetMetadata {
  name: string;
  /** One-word labels to organize and find budgets (at most 10, no spaces). */
  labels?: string[];
  annotations?: Record<string, string>;
}

export interface BudgetCurrentPeriod {
  periodStart: string;
  spent: string;
  unpricedRequests: number;
}

export interface BudgetServerStatus {
  origin: "controlPlane" | "gatewayApi";
  currentPeriod?: BudgetCurrentPeriod;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
}

/** A budget as the Budget kind; status is set by the server. */
export interface Budget {
  apiVersion: typeof BUDGET_API_VERSION;
  kind: "Budget";
  metadata: BudgetMetadata;
  spec: BudgetSpec;
  status?: BudgetServerStatus;
}

export interface BudgetListResponse extends PaginationMeta {
  budgets: Budget[];
}

export interface BudgetAllowanceUsage {
  /** The values this allowance counts, e.g. { agent: "hotel-booking" }; empty when shared. */
  allowance: Record<string, string>;
  spent: string;
  unpricedRequests: number;
}

export interface BudgetUsageListResponse extends PaginationMeta {
  periodStart: string;
  allowances: BudgetAllowanceUsage[];
}

export type ListBudgetsPathParams = OrgPathParams;
export interface ListBudgetsQuery extends ListQuery {
  /** Only the budgets this project owns. */
  project?: string;
  filter?: string;
  /** Budgets whose name, display name or a label contains this text, ignoring case. */
  search?: string;
  status?: "active" | "inactive" | "limitReached";
}

export interface BudgetPathParams extends OrgPathParams {
  budgetName: string | undefined;
}

export type CostGroupBy =
  | "organization"
  | "project"
  | "agent"
  | "apiKey"
  | "user"
  | "llmProvider"
  | "model";

/** One month's cost; a month without traffic is "0.00". */
export interface MonthlyCost {
  /** UTC calendar month, YYYY-MM. */
  month: string;
  cost: string;
}

/** Requests left out of cost over the kept months, by reason. */
export interface UnpricedRequests {
  /** No price for the model. */
  missingPrice: number;
  /** The provider reported no token usage. */
  usageUnavailable: number;
}

/** What one value of groupBy cost. Amounts are decimal strings. */
export interface CostByGroup {
  /** The groupBy value; null when requests carried none (e.g. no user). */
  id: string | null;
  /** The agent's project; set only when grouped by agent. */
  project?: string;
  /** Total over the kept months. */
  cost: string;
  unpricedRequests: UnpricedRequests;
  /** The kept months, oldest first; the last is the current month so far. */
  monthly: MonthlyCost[];
}

export interface CostReport {
  currency: string;
  /** One entry per value of groupBy, highest cost first. */
  costByGroup: CostByGroup[];
}

export type GetCostsPathParams = OrgPathParams;
export interface GetCostsQuery {
  groupBy?: CostGroupBy;
  /** `type:value` filters; repeat for AND, a comma list inside one is OR. */
  filter?: string[];
}

/** What a CSV import does with a row naming a budget that exists. */
export type BudgetImportOnExisting = "fail" | "skip" | "update";

export interface ImportBudgetsQuery {
  /** Check every row and save nothing. */
  dryRun?: boolean;
  onExisting?: BudgetImportOnExisting;
}

export interface BudgetImportRowError {
  /** The CSV column; absent when the error is about the whole row. */
  column?: string;
  code: string;
  message: string;
}

export interface BudgetImportRowResult {
  /** CSV row number; the header is row 1. */
  row: number;
  name?: string;
  /** What a valid row does, or did when applied. */
  action?: "create" | "update" | "skip";
  errors?: BudgetImportRowError[];
}

export interface BudgetImportSummary {
  valid: number;
  error: number;
  created: number;
  updated: number;
  skipped: number;
}

export interface BudgetImportResult {
  dryRun: boolean;
  /** True when the budgets were saved. */
  applied: boolean;
  summary: BudgetImportSummary;
  results: BudgetImportRowResult[];
}

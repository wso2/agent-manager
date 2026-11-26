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

import { useAuthHooks } from "@agent-management-platform/auth";
import { useQueryClient } from "@tanstack/react-query";
import type {
  Budget,
  BudgetListResponse,
  BudgetImportOnExisting,
  BudgetImportResult,
  BudgetPathParams,
  BudgetUsageListResponse,
  CostReport,
  GetCostsPathParams,
  GetCostsQuery,
  ListBudgetsPathParams,
  ListBudgetsQuery,
  ListQuery,
} from "@agent-management-platform/types";
import {
  createBudget,
  deleteBudget,
  getCosts,
  getBudget,
  getBudgetUsage,
  importBudgets,
  listBudgets,
  updateBudget,
} from "../apis/budgets";
import {
  extractServerErrorMessage,
  MAX_SNACKBAR_REASON_LENGTH,
  useApiMutation,
  useApiQuery,
} from "../../shared/react-query-notifications";

/** True for a request that never reached the server (browser "Failed to fetch"). */
export function isNetworkError(error: unknown): boolean {
  const message = (error as { message?: unknown } | null)?.message;
  return (
    error instanceof TypeError ||
    (typeof message === "string" && /failed to fetch|networkerror|load failed/i.test(message))
  );
}

/**
 * The message for a budget change that failed: a plain one when the server could not
 * be reached (never the browser's "Failed to fetch"), else what the server said.
 */
function budgetErrorMessage(verb: string) {
  return (error: unknown) =>
    isNetworkError(error)
      ? `Couldn't ${verb} the budget: the server can't be reached. Try again.`
      : (extractServerErrorMessage(error, { maxReasonLength: MAX_SNACKBAR_REASON_LENGTH }) ??
        `Couldn't ${verb} the budget.`);
}
import { useUIModulePresent } from "../../shared/useUIModulePresent";

export function useListBudgets(params: ListBudgetsPathParams, query?: ListBudgetsQuery) {
  const { getToken } = useAuthHooks();
  const gatewayPresent = useUIModulePresent("ai-gateway");
  return useApiQuery<BudgetListResponse>({
    queryKey: ["budgets", params, query],
    queryFn: () => listBudgets(params, query, getToken),
    enabled: gatewayPresent && !!params.orgName,
  });
}

export function useGetBudget(params: BudgetPathParams) {
  const { getToken } = useAuthHooks();
  const gatewayPresent = useUIModulePresent("ai-gateway");
  return useApiQuery<Budget>({
    queryKey: ["budget", params],
    queryFn: () => getBudget(params, getToken),
    enabled: gatewayPresent && !!params.orgName && !!params.budgetName,
  });
}

export function useBudgetUsage(params: BudgetPathParams, query?: ListQuery) {
  const { getToken } = useAuthHooks();
  const gatewayPresent = useUIModulePresent("ai-gateway");
  return useApiQuery<BudgetUsageListResponse>({
    queryKey: ["budget-usage", params, query],
    queryFn: () => getBudgetUsage(params, query, getToken),
    enabled: gatewayPresent && !!params.orgName && !!params.budgetName,
  });
}

export function useCreateBudget() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    Budget,
    unknown,
    { params: ListBudgetsPathParams; body: Budget }
  >({
    action: { verb: "create", target: "budget" },
    errorMessage: budgetErrorMessage("create"),
    mutationFn: ({ params, body }) => createBudget(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["budgets"] });
    },
  });
}

export function useUpdateBudget() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    Budget,
    unknown,
    { params: BudgetPathParams; body: Budget }
  >({
    action: { verb: "update", target: "budget" },
    errorMessage: budgetErrorMessage("update"),
    mutationFn: ({ params, body }) => updateBudget(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ["budgets"] });
      queryClient.invalidateQueries({ queryKey: ["budget", params] });
    },
  });
}

export function useDeleteBudget() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, BudgetPathParams>({
    action: { verb: "delete", target: "budget" },
    errorMessage: budgetErrorMessage("delete"),
    mutationFn: (params) => deleteBudget(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["budgets"] });
    },
  });
}

export function useCosts(params: GetCostsPathParams, query?: GetCostsQuery) {
  const { getToken } = useAuthHooks();
  const gatewayPresent = useUIModulePresent("ai-gateway");
  return useApiQuery<CostReport>({
    queryKey: ["costs", params, query],
    queryFn: () => getCosts(params, query, getToken),
    enabled: gatewayPresent && !!params.orgName,
  });
}

/** A CSV file to import, and what to do with rows naming budgets that exist. */
export interface BudgetImportRequest {
  csv: string;
  onExisting: BudgetImportOnExisting;
}

/** Checks a CSV file without saving anything (dryRun); its outcome is shown in place. */
export function useCheckBudgetImport(params: ListBudgetsPathParams) {
  const { getToken } = useAuthHooks();
  return useApiMutation<BudgetImportResult, unknown, BudgetImportRequest>({
    showSuccess: false,
    showError: false,
    mutationFn: ({ csv, onExisting }) =>
      importBudgets(params, csv, { dryRun: true, onExisting }, getToken),
  });
}

/** Imports the budgets of a CSV file; errors are shown in place. */
export function useImportBudgets(params: ListBudgetsPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<BudgetImportResult, unknown, BudgetImportRequest>({
    showError: false,
    successMessage: (result: BudgetImportResult) => {
      const { created, updated, skipped } = result.summary;
      const parts = [
        created ? `${created} created` : "",
        updated ? `${updated} updated` : "",
        skipped ? `${skipped} skipped` : "",
      ].filter(Boolean);
      return `Budgets imported: ${parts.join(", ") || "nothing to change"}.`;
    },
    mutationFn: ({ csv, onExisting }) =>
      importBudgets(params, csv, { onExisting }, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["budgets"] });
    },
  });
}

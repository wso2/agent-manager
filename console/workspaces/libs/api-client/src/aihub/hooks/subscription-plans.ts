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

import { keepPreviousData, useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import type {
  CreateSubscriptionPlanRequest,
  ListSubscriptionPlansPathParams,
  ListSubscriptionPlansQuery,
  SubscriptionPlan,
  SubscriptionPlanListResponse,
  SubscriptionPlanPathParams,
  UpdateSubscriptionPlanRequest,
} from "@agent-management-platform/types";
import { useUIModulePresent } from "../../shared/useUIModulePresent";
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";
import {
  createSubscriptionPlan,
  deleteSubscriptionPlan,
  getSubscriptionPlan,
  listSubscriptionPlans,
  updateSubscriptionPlan,
} from "../apis/subscription-plans";

export const subscriptionPlanKeys = {
  lists: () => ["subscription-plans"] as const,
  list: (params: ListSubscriptionPlansPathParams, query?: ListSubscriptionPlansQuery) =>
    ["subscription-plans", params, query] as const,
  details: () => ["subscription-plan"] as const,
  detail: (params: SubscriptionPlanPathParams) => ["subscription-plan", params] as const,
};

/** The code the API answers a new plan whose handle is taken with. */
export const SUBSCRIPTION_PLAN_EXISTS = "SUBSCRIPTION_PLAN_EXISTS";

/** The API error code of a failed request, from the error the http helpers throw. */
export function subscriptionPlanErrorCode(error: unknown): string | undefined {
  const code = (error as { body?: { code?: unknown } } | null)?.body?.code;
  return typeof code === "string" ? code : undefined;
}

/**
 * Subscription plans belong to the AI Hub module. Without it (left out by the server UI
 * manifest, or not installed) every query below stays idle, wherever it is called from.
 */
function useAIHubPresent(): boolean {
  return useUIModulePresent("aihub");
}

export function useListSubscriptionPlans(
  params: ListSubscriptionPlansPathParams,
  query?: ListSubscriptionPlansQuery,
) {
  const { getToken } = useAuthHooks();
  const present = useAIHubPresent();
  return useApiQuery<SubscriptionPlanListResponse>({
    queryKey: subscriptionPlanKeys.list(params, query),
    queryFn: () => listSubscriptionPlans(params, query, getToken),
    enabled: present && !!params.orgName,
    // Paging and searching keep the current rows until the next page arrives.
    placeholderData: keepPreviousData,
  });
}

/** One plan, with its usage (what offers or uses it). `planId` undefined keeps it idle. */
export function useGetSubscriptionPlan(params: { orgName?: string; planId?: string }) {
  const { getToken } = useAuthHooks();
  const present = useAIHubPresent();
  return useApiQuery<SubscriptionPlan>({
    queryKey: subscriptionPlanKeys.detail({ orgName: params.orgName, planId: params.planId ?? "" }),
    queryFn: () =>
      getSubscriptionPlan({ orgName: params.orgName, planId: params.planId ?? "" }, getToken),
    enabled: present && !!params.orgName && !!params.planId,
  });
}

/** Creates a plan. Errors are left to the form: a taken handle is shown on the name field. */
export function useCreateSubscriptionPlan() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    SubscriptionPlan,
    unknown,
    { params: ListSubscriptionPlansPathParams; body: CreateSubscriptionPlanRequest }
  >({
    successMessage: (plan) => `Created plan "${plan.displayName}".`,
    showError: false,
    mutationFn: ({ params, body }) => createSubscriptionPlan(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: subscriptionPlanKeys.lists() });
    },
  });
}

export function useUpdateSubscriptionPlan() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    SubscriptionPlan,
    unknown,
    {
      params: SubscriptionPlanPathParams;
      body: UpdateSubscriptionPlanRequest;
      /** The success message to show, e.g. for a status switch. */
      successText?: string;
    }
  >({
    successMessage: (plan, variables) => variables.successText ?? `Updated plan "${plan.displayName}".`,
    errorMessage: (error) =>
      (error as { message?: string } | null)?.message || "Failed to update the plan.",
    mutationFn: ({ params, body }) => updateSubscriptionPlan(params, body, getToken),
    onSuccess: (plan) => {
      // Put the saved plan into the cached lists first, so a row shows its new state at once and
      // doesn't flip back while the lists reload.
      queryClient.setQueriesData<SubscriptionPlanListResponse>(
        { queryKey: subscriptionPlanKeys.lists() },
        (list) =>
          list && {
            ...list,
            list: list.list.map((item) => (item.id === plan.id ? { ...item, ...plan } : item)),
          },
      );
      queryClient.invalidateQueries({ queryKey: subscriptionPlanKeys.lists() });
      queryClient.invalidateQueries({ queryKey: subscriptionPlanKeys.details() });
    },
  });
}

export function useDeleteSubscriptionPlan() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, SubscriptionPlanPathParams & { displayName: string }>({
    successMessage: (_data, variables) => `Deleted plan "${variables.displayName}".`,
    errorMessage: (error) =>
      (error as { message?: string } | null)?.message || "Failed to delete the plan.",
    mutationFn: (params) => deleteSubscriptionPlan(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: subscriptionPlanKeys.lists() });
      queryClient.invalidateQueries({ queryKey: subscriptionPlanKeys.details() });
    },
  });
}

/** Reads one plan's usage now, for a delete that must say whether the plan is in use. */
export function useFetchSubscriptionPlan() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return (params: SubscriptionPlanPathParams) =>
    queryClient.fetchQuery({
      queryKey: subscriptionPlanKeys.detail(params),
      queryFn: () => getSubscriptionPlan(params, getToken),
      staleTime: 0,
    });
}

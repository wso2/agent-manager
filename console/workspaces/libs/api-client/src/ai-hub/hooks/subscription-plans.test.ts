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

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const snack = vi.hoisted(() => ({ push: vi.fn() }));
const modules = vi.hoisted(() => ({ installed: new Set(["ai-hub"]) }));
vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: snack.push }),
  useIsUIModuleInstalled: (id: string) => modules.installed.has(id),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/subscription-plans", () => ({
  listSubscriptionPlans: vi.fn(),
  getSubscriptionPlan: vi.fn(),
  createSubscriptionPlan: vi.fn(),
  updateSubscriptionPlan: vi.fn(),
  deleteSubscriptionPlan: vi.fn(),
}));

import type { SubscriptionPlanListResponse } from "@agent-management-platform/types";
import type { UseQueryResult } from "@tanstack/react-query";
import {
  createSubscriptionPlan,
  deleteSubscriptionPlan,
  getSubscriptionPlan,
  listSubscriptionPlans,
  updateSubscriptionPlan,
} from "../apis/subscription-plans";
import { cleanupHooks, flush, renderHook, runMutation } from "../../test-utils/query-hooks";
import {
  SUBSCRIPTION_PLAN_EXISTS,
  subscriptionPlanErrorCode,
  subscriptionPlanKeys,
  useCreateSubscriptionPlan,
  useDeleteSubscriptionPlan,
  useGetSubscriptionPlan,
  useListSubscriptionPlans,
  useUpdateSubscriptionPlan,
} from "./subscription-plans";

const org = { orgName: "o" };
const SEEDED = {
  list: subscriptionPlanKeys.list(org),
  detail: subscriptionPlanKeys.detail({ orgName: "o", planId: "gold" }),
};

beforeEach(() => {
  snack.push.mockReset();
  vi.mocked(listSubscriptionPlans).mockReset().mockResolvedValue({ list: [], count: 0 } as never);
  vi.mocked(getSubscriptionPlan).mockReset().mockResolvedValue({ id: "gold" } as never);
  vi.mocked(createSubscriptionPlan).mockReset().mockResolvedValue({ id: "gold", displayName: "Gold" } as never);
  vi.mocked(updateSubscriptionPlan).mockReset().mockResolvedValue({ id: "gold", displayName: "Gold 2", status: "INACTIVE" } as never);
  vi.mocked(deleteSubscriptionPlan).mockReset().mockResolvedValue(undefined);
});

afterEach(() => {
  cleanupHooks();
  modules.installed.add("ai-hub");
});

async function settle<T>(use: () => T) {
  const { result, qc } = await renderHook(use);
  await flush();
  return { query: result.current as unknown as UseQueryResult, qc };
}

describe("subscriptionPlanErrorCode", () => {
  it.each([
    [{ body: { code: SUBSCRIPTION_PLAN_EXISTS } }, SUBSCRIPTION_PLAN_EXISTS],
    [{ body: { code: 409 } }, undefined],
    [{ body: {} }, undefined],
    [null, undefined],
  ])("reads the API error code of %j", (error, expected) => {
    expect(subscriptionPlanErrorCode(error)).toBe(expected);
  });
});

describe("plan queries", () => {
  it("lists plans with the query", async () => {
    const query = { query: "go", limit: 10 } as never;
    await settle(() => useListSubscriptionPlans(org, query));
    expect(listSubscriptionPlans).toHaveBeenCalledWith(org, query, expect.any(Function));
  });

  it("gets one plan", async () => {
    const { qc } = await settle(() => useGetSubscriptionPlan({ orgName: "o", planId: "gold" }));
    expect(getSubscriptionPlan).toHaveBeenCalledWith({ orgName: "o", planId: "gold" }, expect.any(Function));
    expect(qc.getQueryData(SEEDED.detail)).toEqual({ id: "gold" });
  });

  it.each([
    ["no plan", () => useGetSubscriptionPlan({ orgName: "o" })],
    ["no org", () => useListSubscriptionPlans({ orgName: "" } as never)],
    ["the AI Hub absent", () => { modules.installed.clear(); return useListSubscriptionPlans(org); }],
  ])("stays idle with %s", async (_n, use) => {
    const { query } = await settle(use as () => unknown);
    expect(query.fetchStatus).toBe("idle");
    expect(listSubscriptionPlans).not.toHaveBeenCalled();
    expect(getSubscriptionPlan).not.toHaveBeenCalled();
  });
});

describe("plan mutations", () => {
  it("creates a plan, reloads the lists and leaves errors to the form", async () => {
    const { invalidated } = await runMutation(useCreateSubscriptionPlan, { params: org, body: { id: "gold" } }, SEEDED);
    expect(createSubscriptionPlan).toHaveBeenCalledWith(org, { id: "gold" }, expect.any(Function));
    expect(invalidated).toEqual(["list"]);
    expect(snack.push).toHaveBeenCalledWith(expect.objectContaining({ message: 'Created plan "Gold".' }));

    snack.push.mockReset();
    vi.mocked(createSubscriptionPlan).mockRejectedValue(Object.assign(new Error("taken"), { body: { code: SUBSCRIPTION_PLAN_EXISTS } }));
    const { error } = await runMutation(useCreateSubscriptionPlan, { params: org, body: { id: "gold" } });
    expect(subscriptionPlanErrorCode(error)).toBe(SUBSCRIPTION_PLAN_EXISTS);
    expect(snack.push).not.toHaveBeenCalled();
  });

  it("updates a plan, merging it into the cached lists at once and reloading lists and details", async () => {
    const { qc, result } = await renderHook(useUpdateSubscriptionPlan);
    qc.setQueryData<SubscriptionPlanListResponse>(SEEDED.list, {
      list: [{ id: "gold", displayName: "Gold", status: "ACTIVE" }, { id: "silver", displayName: "Silver" }],
      count: 2,
    } as never);
    qc.setQueryData(SEEDED.detail, { id: "gold" });
    await act(async () => {
      await result.current.mutateAsync({ params: { orgName: "o", planId: "gold" }, body: { status: "INACTIVE" } } as never);
    });
    const list = qc.getQueryData<SubscriptionPlanListResponse>(SEEDED.list);
    expect(list?.list.map((p) => [p.id, p.displayName, p.status])).toEqual([
      ["gold", "Gold 2", "INACTIVE"],
      ["silver", "Silver", undefined],
    ]);
    expect(snack.push).toHaveBeenCalledWith(expect.objectContaining({ message: 'Updated plan "Gold 2".' }));
    expect(qc.getQueryState(SEEDED.list)?.isInvalidated).toBe(true);
    expect(qc.getQueryState(SEEDED.detail)?.isInvalidated).toBe(true);
  });

  it("shows the success text it is given", async () => {
    await runMutation(useUpdateSubscriptionPlan, {
      params: { orgName: "o", planId: "gold" }, body: { status: "INACTIVE" }, successText: "Deactivated plan.",
    });
    expect(snack.push).toHaveBeenCalledWith(expect.objectContaining({ message: "Deactivated plan." }));
  });

  it.each([
    [new Error("Plan is in use"), "Plan is in use"],
    [{}, "Failed to update the plan."],
  ])("shows the update error %#", async (failure, message) => {
    vi.mocked(updateSubscriptionPlan).mockRejectedValue(failure);
    await runMutation(useUpdateSubscriptionPlan, { params: { orgName: "o", planId: "gold" }, body: {} });
    expect(snack.push).toHaveBeenCalledWith(expect.objectContaining({ message }));
  });

  it("deletes a plan and reloads lists and details", async () => {
    const { invalidated } = await runMutation(
      useDeleteSubscriptionPlan, { orgName: "o", planId: "gold", displayName: "Gold" }, SEEDED,
    );
    expect(deleteSubscriptionPlan).toHaveBeenCalledWith(
      { orgName: "o", planId: "gold", displayName: "Gold" }, expect.any(Function),
    );
    expect(invalidated).toEqual(["detail", "list"]);
    expect(snack.push).toHaveBeenCalledWith(expect.objectContaining({ message: 'Deleted plan "Gold".' }));
  });

  it.each([
    [new Error("Plan is in use"), "Plan is in use"],
    [{}, "Failed to delete the plan."],
  ])("shows the delete error %#", async (failure, message) => {
    vi.mocked(deleteSubscriptionPlan).mockRejectedValue(failure);
    await runMutation(useDeleteSubscriptionPlan, { orgName: "o", planId: "gold", displayName: "Gold" });
    expect(snack.push).toHaveBeenCalledWith(expect.objectContaining({ message }));
  });
});

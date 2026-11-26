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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const snack = vi.hoisted(() => ({ pushSnackBar: vi.fn() }));
const api = vi.hoisted(() => ({
  listBudgets: vi.fn(async () => ({ budgets: [], total: 0 })),
  getBudget: vi.fn(async () => ({ metadata: { name: "org-daily" } })),
  getBudgetUsage: vi.fn(async () => ({ usages: [] })),
  getCosts: vi.fn(async () => ({ costByGroup: [] })),
  createBudget: vi.fn(async () => ({})),
  updateBudget: vi.fn(async () => ({})),
  deleteBudget: vi.fn(async () => undefined),
  importBudgets: vi.fn(async () => ({ summary: { created: 2, updated: 1, skipped: 0 } })),
}));

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => snack,
  useIsUIModuleInstalled: () => true,
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/budgets", () => api);

import {
  isNetworkError,
  useBudgetUsage,
  useCheckBudgetImport,
  useCosts,
  useCreateBudget,
  useDeleteBudget,
  useGetBudget,
  useImportBudgets,
  useListBudgets,
  useUpdateBudget,
} from "./budgets";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const org = { orgName: "acme" };
const budget = { orgName: "acme", budgetName: "org-daily" };
const SEEDED = {
  budgets: ["budgets", org],
  budget: ["budget", budget],
  costs: ["costs", org],
} as const;

async function render<T>(useHook: () => T, qc = new QueryClient()) {
  const result = {} as { current: T };
  const Probe = () => {
    result.current = useHook();
    return null;
  };
  const root = createRoot(document.createElement("div"));
  await act(async () =>
    root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
  );
  return { result, qc };
}

async function invalidatedAfter(
  useHook: () => { mutateAsync: (v: never) => Promise<unknown> },
  variables: unknown,
): Promise<string[]> {
  const qc = new QueryClient();
  Object.values(SEEDED).forEach((key) => qc.setQueryData(key, {}));
  const { result } = await render(useHook, qc);
  await act(async () => { await result.current.mutateAsync(variables as never); });
  return Object.entries(SEEDED)
    .filter(([, key]) => qc.getQueryState(key)?.isInvalidated)
    .map(([name]) => name);
}

beforeEach(() => {
  snack.pushSnackBar.mockReset();
  Object.values(api).forEach((fn) => fn.mockClear());
});

describe("budget queries", () => {
  it("read with their params and the token getter", async () => {
    await render(() => useListBudgets(org, { filter: "project:online-shop" } as never));
    await render(() => useGetBudget(budget));
    await render(() => useBudgetUsage(budget, { limit: 5 }));
    await render(() => useCosts(org, { groupBy: "agent" } as never));
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    expect(api.listBudgets).toHaveBeenCalledWith(org, { filter: "project:online-shop" }, expect.any(Function));
    expect(api.getBudget).toHaveBeenCalledWith(budget, expect.any(Function));
    expect(api.getBudgetUsage).toHaveBeenCalledWith(budget, { limit: 5 }, expect.any(Function));
    expect(api.getCosts).toHaveBeenCalledWith(org, { groupBy: "agent" }, expect.any(Function));
  });

  it("wait for an organization and a budget name", async () => {
    await render(() => useListBudgets({ orgName: "" }));
    await render(() => useGetBudget({ orgName: "acme", budgetName: "" }));
    await render(() => useCosts({ orgName: "" }));
    expect(api.listBudgets).not.toHaveBeenCalled();
    expect(api.getBudget).not.toHaveBeenCalled();
    expect(api.getCosts).not.toHaveBeenCalled();
  });
});

describe("budget mutations", () => {
  it("create refreshes the lists", async () => {
    expect(await invalidatedAfter(useCreateBudget, { params: org, body: {} })).toEqual(["budgets"]);
  });

  it("update refreshes the lists and the budget", async () => {
    expect(await invalidatedAfter(useUpdateBudget, { params: budget, body: {} })).toEqual(["budgets", "budget"]);
  });

  it("delete refreshes the lists", async () => {
    expect(await invalidatedAfter(useDeleteBudget, budget)).toEqual(["budgets"]);
  });

  it("import refreshes the lists and says what changed", async () => {
    expect(await invalidatedAfter(() => useImportBudgets(org), { csv: "x", onExisting: "update" })).toEqual(["budgets"]);
    expect(api.importBudgets).toHaveBeenCalledWith(org, "x", { onExisting: "update" }, expect.any(Function));
    expect(JSON.stringify(snack.pushSnackBar.mock.calls)).toContain("2 created, 1 updated");
  });

  it("a check of the file is a dry run and saves nothing", async () => {
    const { result } = await render(() => useCheckBudgetImport(org));
    await act(async () => { await result.current.mutateAsync({ csv: "x", onExisting: "fail" }); });
    expect(api.importBudgets).toHaveBeenCalledWith(org, "x", { dryRun: true, onExisting: "fail" }, expect.any(Function));
  });

  it("says plainly when the server can't be reached", async () => {
    api.deleteBudget.mockRejectedValueOnce(new TypeError("Failed to fetch"));
    const { result } = await render(useDeleteBudget);
    await act(async () => {
      await result.current.mutateAsync(budget as never).catch(() => undefined);
    });
    const shown = JSON.stringify(snack.pushSnackBar.mock.calls);
    expect(shown).toContain("the server can't be reached");
    expect(shown).not.toContain("Failed to fetch");
  });
});

describe("isNetworkError", () => {
  it("knows a request that never reached the server", () => {
    expect(isNetworkError(new TypeError("x"))).toBe(true);
    expect(isNetworkError({ message: "NetworkError when attempting to fetch" })).toBe(true);
    expect(isNetworkError({ message: "Load failed" })).toBe(true);
    expect(isNetworkError({ message: "budget not found" })).toBe(false);
    expect(isNetworkError(null)).toBe(false);
  });
});

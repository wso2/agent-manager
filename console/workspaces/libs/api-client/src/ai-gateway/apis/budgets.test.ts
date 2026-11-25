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

import { afterEach, describe, expect, it, vi } from "vitest";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, getToken, type HttpMocks } from "../../test-utils/http-cases";
import {
  createBudget,
  deleteBudget,
  getBudget,
  getBudgetUsage,
  getCosts,
  importBudgets,
  listBudgets,
  updateBudget,
} from "./budgets";

const auth = vi.hoisted(() => ({ backendFetch: vi.fn() }));
vi.mock("@agent-management-platform/auth", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  backendFetch: auth.backendFetch,
}));
vi.mock("@agent-management-platform/types", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  globalConfig: { apiBaseUrl: "/api" },
}));
vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const ORG = "/m/ai-gateway/orgs/acme%20corp";
const org = { orgName: "acme corp" };
const budget = { orgName: "acme corp", budgetName: "org-daily" };
const kind = { apiVersion: "gateway.api-platform.wso2.com/v1", kind: "Budget", metadata: { name: "org-daily" } };
const httpMocks = {
  GET: vi.mocked(httpGET),
  POST: vi.mocked(httpPOST),
  PUT: vi.mocked(httpPUT),
  DELETE: vi.mocked(httpDELETE),
} as unknown as HttpMocks;

describeHttpCases("budgets API", httpMocks, [
  {
    name: "listBudgets forwards every filter",
    call: () => listBudgets(org, {
      project: "online-shop", filter: "llmProvider:mock-openai", search: "daily", status: "active", limit: 10, offset: 20,
    } as never, getToken),
    method: "GET", url: `${ORG}/budgets`,
    searchParams: {
      project: "online-shop", filter: "llmProvider:mock-openai", search: "daily", status: "active", limit: "10", offset: "20",
    },
  },
  {
    name: "listBudgets without filters sends no search params",
    call: () => listBudgets(org, undefined, getToken),
    method: "GET", url: `${ORG}/budgets`, searchParams: {},
  },
  { name: "getBudget", call: () => getBudget(budget, getToken), method: "GET", url: `${ORG}/budgets/org-daily` },
  {
    name: "createBudget",
    call: () => createBudget(org, kind as never, getToken),
    method: "POST", url: `${ORG}/budgets`, body: kind,
  },
  {
    name: "updateBudget",
    call: () => updateBudget(budget, kind as never, getToken),
    method: "PUT", url: `${ORG}/budgets/org-daily`, body: kind,
  },
  {
    name: "deleteBudget",
    call: () => deleteBudget(budget, getToken),
    method: "DELETE", url: `${ORG}/budgets/org-daily`, returnsBody: false,
  },
  {
    name: "getBudgetUsage pages the allowances",
    call: () => getBudgetUsage(budget, { limit: 5, offset: 0 }, getToken),
    method: "GET", url: `${ORG}/budgets/org-daily/usage`, searchParams: { limit: "5", offset: "0" },
  },
  {
    name: "getCosts repeats filter for AND",
    call: () => getCosts(org, { groupBy: "agent", filter: ["project:online-shop", "llmProvider:mock-openai"] } as never, getToken),
    method: "GET", url: `${ORG}/costs`,
    searchParams: [["groupBy", "agent"], ["filter", "project:online-shop"], ["filter", "llmProvider:mock-openai"]],
  },
]);

describe("importBudgets", () => {
  afterEach(() => auth.backendFetch.mockReset());
  const csv = "name,amount\norg-daily,15.00\n";
  const result = { dryRun: true, applied: false, summary: { valid: 1, error: 0 }, results: [] };

  it("posts the file as text/csv with the token and options", async () => {
    auth.backendFetch.mockResolvedValue(new Response(JSON.stringify(result), { status: 200 }));
    await expect(importBudgets(org, csv, { dryRun: true, onExisting: "update" } as never, getToken)).resolves.toEqual(result);
    const [url, init] = auth.backendFetch.mock.calls[0];
    expect(url).toBe(`/api${ORG}/budgets/import?dryRun=true&onExisting=update`);
    expect(init).toEqual({
      method: "POST", headers: { "Content-Type": "text/csv", Authorization: "Bearer jwt" }, body: csv,
    });
  });

  it("returns a file with row errors like a success", async () => {
    const withErrors = {
      ...result, summary: { valid: 0, error: 1 }, results: [{ row: 2, errors: [] }],
    };
    auth.backendFetch.mockResolvedValue(new Response(JSON.stringify(withErrors), { status: 400 }));
    await expect(importBudgets(org, csv)).resolves.toEqual(withErrors);
  });

  it("throws any other error body", async () => {
    auth.backendFetch.mockResolvedValue(new Response(JSON.stringify({ code: "INVALID_CSV" }), { status: 400 }));
    await expect(importBudgets(org, csv)).rejects.toEqual({ code: "INVALID_CSV" });
  });

  it("throws the status when the body is not JSON", async () => {
    auth.backendFetch.mockResolvedValue(new Response("bad gateway", { status: 502 }));
    await expect(importBudgets(org, csv)).rejects.toMatchObject({ status: 502 });
  });
});

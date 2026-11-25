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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import {
  createSubscriptionPlan,
  deleteSubscriptionPlan,
  getSubscriptionPlan,
  listSubscriptionPlans,
  updateSubscriptionPlan,
} from "./subscription-plans";

vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const PLANS = "/m/ai-hub/subscription-plans";
const getToken = async () => "jwt";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const noContent = () => new Response(null, { status: 204 });

beforeEach(() => {
  vi.mocked(httpGET).mockReset().mockImplementation(async () => json({ list: [] }));
  vi.mocked(httpPOST).mockReset().mockImplementation(async () => json({ id: "gold" }, 201));
  vi.mocked(httpPUT).mockReset().mockImplementation(async () => json({ id: "gold" }));
  vi.mocked(httpDELETE).mockReset().mockImplementation(async () => noContent());
});

describe("subscription plans API", () => {
  it("lists plans for the caller's organization, sending only the query values that are set", async () => {
    const query = { query: "go", limit: 10, offset: 0, status: "" } as never;
    await listSubscriptionPlans({ orgName: "acme" }, query, getToken);
    expect(httpGET).toHaveBeenCalledWith(PLANS, {
      searchParams: { query: "go", limit: "10", offset: "0" },
      token: "jwt",
    });
  });

  it("lists without a token or a query", async () => {
    await expect(listSubscriptionPlans({ orgName: "acme" })).resolves.toEqual({ list: [] });
    expect(httpGET).toHaveBeenCalledWith(PLANS, { searchParams: {}, token: undefined });
  });

  it("gets one plan by its encoded handle", async () => {
    vi.mocked(httpGET).mockResolvedValue(json({ id: "a b" }));
    await expect(getSubscriptionPlan({ orgName: "acme", planId: "a b" }, getToken)).resolves.toEqual({ id: "a b" });
    expect(httpGET).toHaveBeenCalledWith(`${PLANS}/a%20b`, { token: "jwt" });
  });

  it.each([
    [json({ message: "Plan not found" }, 404), "Plan not found", { message: "Plan not found" }],
    [new Response("not json", { status: 500, statusText: "Server Error" }), "Request failed: 500 Server Error", undefined],
  ])("fails with the server's message, status and body", async (response, message, body) => {
    vi.mocked(httpGET).mockResolvedValue(response);
    await expect(getSubscriptionPlan({ orgName: "acme", planId: "gold" }, getToken)).rejects.toMatchObject({
      message,
      status: response.status,
      body,
    });
  });

  it("fails a list with the server's message", async () => {
    vi.mocked(httpGET).mockResolvedValue(json({ message: "no" }, 403));
    await expect(listSubscriptionPlans({ orgName: "acme" }, undefined, getToken)).rejects.toMatchObject({ status: 403 });
  });

  it("creates, updates and deletes a plan", async () => {
    const body = { id: "gold", displayName: "Gold" };
    await expect(createSubscriptionPlan({ orgName: "acme" }, body as never, getToken)).resolves.toEqual({ id: "gold" });
    expect(httpPOST).toHaveBeenCalledWith(PLANS, body, { token: "jwt" });

    await updateSubscriptionPlan({ orgName: "acme", planId: "gold" }, { displayName: "Gold 2" } as never, getToken);
    expect(httpPUT).toHaveBeenCalledWith(`${PLANS}/gold`, { displayName: "Gold 2" }, { token: "jwt" });

    await deleteSubscriptionPlan({ orgName: "acme", planId: "gold" });
    expect(httpDELETE).toHaveBeenCalledWith(`${PLANS}/gold`, { token: undefined });
  });
});

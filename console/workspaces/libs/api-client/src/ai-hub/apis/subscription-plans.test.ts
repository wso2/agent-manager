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
  deprecateSubscriptionPlan,
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
const DOC = { apiVersion: "ai-control-plane.choreolabs.com/v1", kind: "SubscriptionPlan" };
const doc = (
  name: string,
  spec: Record<string, unknown> = { displayName: name },
  status?: unknown,
) => ({
  ...DOC,
  metadata: { name },
  spec,
  ...(status ? { status } : {}),
});

beforeEach(() => {
  vi.mocked(httpGET).mockReset().mockImplementation(async () => json({ list: [] }));
  vi.mocked(httpPOST).mockReset().mockImplementation(async () => json(doc("gold"), 201));
  vi.mocked(httpPUT).mockReset().mockImplementation(async () => json(doc("gold")));
  vi.mocked(httpDELETE)
    .mockReset()
    .mockImplementation(async () =>
      json(doc("gold", { displayName: "Gold" }, { deprecated: true, deprecationTimestamp: "t3" })),
    );
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

  it("gets one plan by its encoded handle, flattening its document", async () => {
    vi.mocked(httpGET).mockResolvedValue(
      json({
        ...doc(
          "a b",
          { displayName: "A B", status: "INACTIVE", conditions: [{ id: "requests", type: "request-count", params: {} }] },
          { deprecated: false, usage: { inUse: true, mcpProxies: 1, subscriptions: 2 } },
        ),
        metadata: {
          name: "a b", uid: "u-1", createdBy: "alice", creationTimestamp: "t1", updateTimestamp: "t2",
          origin: "ai-control-plane",
        },
      }),
    );
    await expect(getSubscriptionPlan({ orgName: "acme", planId: "a b" }, getToken)).resolves.toEqual({
      id: "a b",
      uid: "u-1",
      displayName: "A B",
      status: "INACTIVE",
      conditions: [{ id: "requests", type: "request-count", params: {} }],
      createdBy: "alice",
      createdAt: "t1",
      updatedAt: "t2",
      usage: { inUse: true, mcpProxies: 1, subscriptions: 2 },
      origin: "ai-control-plane",
      deprecated: false,
    });
    expect(httpGET).toHaveBeenCalledWith(`${PLANS}/a%20b`, { token: "jwt" });
  });

  it("lists plans as flat plans", async () => {
    vi.mocked(httpGET).mockResolvedValue(
      json({ list: [doc("gold")], count: 1, pagination: { total: 1, offset: 0, limit: 20 } }),
    );
    const page = await listSubscriptionPlans({ orgName: "acme" }, undefined, getToken);
    expect(page.list).toEqual([
      expect.objectContaining({ id: "gold", displayName: "gold", status: "ACTIVE", conditions: [] }),
    ]);
    expect(page.pagination).toEqual({ total: 1, offset: 0, limit: 20 });
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

  it("creates, updates and deprecates a plan, sending documents", async () => {
    const body = { id: "gold", displayName: "Gold" };
    await expect(createSubscriptionPlan({ orgName: "acme" }, body as never, getToken)).resolves.toMatchObject({
      id: "gold",
    });
    expect(httpPOST).toHaveBeenCalledWith(PLANS, doc("gold", { displayName: "Gold" }), { token: "jwt" });

    await updateSubscriptionPlan(
      { orgName: "acme", planId: "gold" },
      { displayName: "Gold 2", expiryTime: null } as never,
      getToken,
    );
    expect(httpPUT).toHaveBeenCalledWith(
      `${PLANS}/gold`,
      doc("gold", { displayName: "Gold 2", expiryTime: null }),
      { token: "jwt" },
    );

    await expect(deprecateSubscriptionPlan({ orgName: "acme", planId: "gold" })).resolves.toMatchObject({
      id: "gold",
      deprecated: true,
      deprecatedAt: "t3",
    });
    expect(httpDELETE).toHaveBeenCalledWith(`${PLANS}/gold`, { token: undefined });
  });

  it("fails a deprecation with the server's message", async () => {
    vi.mocked(httpDELETE).mockResolvedValue(json({ message: "gone" }, 404));
    await expect(deprecateSubscriptionPlan({ orgName: "acme", planId: "gold" })).rejects.toMatchObject({
      status: 404,
    });
  });
});

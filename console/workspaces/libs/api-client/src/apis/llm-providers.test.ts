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

import { beforeEach, expect, it, vi } from "vitest";
import { httpGET } from "../utils";
import { listLLMDeployments } from "./llm-providers";

vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils")>()),
  httpGET: vi.fn(),
}));

beforeEach(() => {
  vi.mocked(httpGET).mockReset();
});

it("keeps the filters while following the deployment list's next cursor", async () => {
  const first = { id: "first" };
  const second = { id: "second" };
  vi.mocked(httpGET)
    .mockResolvedValueOnce(Response.json([first], { headers: { "X-Next-Cursor": "opaque-cursor" } }))
    .mockResolvedValueOnce(Response.json([second]));

  const filters = { gatewayId: "edge", status: "DEPLOYED" };
  const result = await listLLMDeployments(
    { orgName: "acme", providerId: "provider" },
    filters,
    async () => "test-token",
  );

  expect(result).toEqual([first, second]);
  const path = "/api/v1/orgs/acme/llm-providers/provider/deployment-history";
  expect(httpGET).toHaveBeenNthCalledWith(1, path, { token: "test-token", searchParams: filters });
  expect(httpGET).toHaveBeenNthCalledWith(2, path, {
    token: "test-token",
    searchParams: { ...filters, cursor: "opaque-cursor" },
  });
  expect(httpGET).toHaveBeenCalledTimes(2);
});

it("lists every deployment when an older server has no deployment history route", async () => {
  const legacyDeployments = [{ id: "original" }, { id: "archived" }];
  vi.mocked(httpGET)
    .mockRejectedValueOnce(Object.assign(new Error("404 page not found"), { status: 404 }))
    .mockResolvedValueOnce(Response.json(legacyDeployments));

  const filters = { gatewayId: "edge", status: "ARCHIVED" };
  const result = await listLLMDeployments(
    { orgName: "acme", providerId: "provider" },
    filters,
    async () => "test-token",
  );

  expect(result).toEqual(legacyDeployments);
  expect(httpGET).toHaveBeenNthCalledWith(1, "/api/v1/orgs/acme/llm-providers/provider/deployment-history", {
    token: "test-token",
    searchParams: filters,
  });
  expect(httpGET).toHaveBeenNthCalledWith(2, "/api/v1/orgs/acme/llm-providers/provider/deployments", {
    token: "test-token",
    searchParams: filters,
  });
});

it("preserves non-404 deployment history errors", async () => {
  const denied = Object.assign(new Error("Forbidden"), { status: 403 });
  vi.mocked(httpGET).mockRejectedValueOnce(denied);

  await expect(listLLMDeployments({ orgName: "acme", providerId: "provider" })).rejects.toBe(denied);
  expect(httpGET).toHaveBeenCalledTimes(1);
});

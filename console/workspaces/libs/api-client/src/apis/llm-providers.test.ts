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

import { expect, it, vi } from "vitest";
import { httpGET } from "../utils";
import { listLLMDeployments } from "./llm-providers";

vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils")>()),
  httpGET: vi.fn(),
}));

it("keeps the filters while following the deployment list's next offset", async () => {
  const first = { id: "first" };
  const second = { id: "second" };
  vi.mocked(httpGET)
    .mockResolvedValueOnce(Response.json([first], { headers: { "X-Next-Offset": "1000" } }))
    .mockResolvedValueOnce(Response.json([second]));

  const filters = { gatewayId: "edge", status: "DEPLOYED" };
  const result = await listLLMDeployments(
    { orgName: "acme", providerId: "provider" },
    filters,
    async () => "test-token",
  );

  expect(result).toEqual([first, second]);
  const path = "/api/v1/orgs/acme/llm-providers/provider/deployments";
  expect(httpGET).toHaveBeenNthCalledWith(1, path, { token: "test-token", searchParams: filters });
  expect(httpGET).toHaveBeenNthCalledWith(2, path, {
    token: "test-token",
    searchParams: { ...filters, offset: "1000" },
  });
  expect(httpGET).toHaveBeenCalledTimes(2);
});

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

import { afterEach, describe, expect, it, vi } from "vitest";
import type * as Utils from "../../shared/utils";

const http = vi.hoisted(() => ({
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));
vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof Utils>()),
  ...http,
}));

import { listCatalogLLMProviders } from "./catalog";

const getToken = async () => "tok";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

afterEach(() => vi.resetAllMocks());

describe("listCatalogLLMProviders", () => {
  it("asks the AI Gateway catalog for LLM providers with every filter given", async () => {
    http.httpGET.mockResolvedValue(json({ entries: [{ id: "p1" }] }));

    await expect(
      listCatalogLLMProviders(
        { orgName: "my org" },
        { limit: 10, offset: 0, environmentId: "env-1", gatewayHosting: "self-hosted" } as never,
        getToken,
      ),
    ).resolves.toEqual({ entries: [{ id: "p1" }] });

    expect(http.httpGET).toHaveBeenCalledWith("/m/ai-gateway/orgs/my%20org/catalog", {
      token: "tok",
      searchParams: {
        kind: "LlmProvider",
        limit: "10",
        offset: "0",
        environmentId: "env-1",
        gatewayHosting: "self-hosted",
      },
    });
  });

  it("sends only the kind without filters", async () => {
    http.httpGET.mockResolvedValue(json({}));
    await listCatalogLLMProviders({ orgName: "o" });
    expect(http.httpGET).toHaveBeenCalledWith("/m/ai-gateway/orgs/o/catalog", {
      token: undefined,
      searchParams: { kind: "LlmProvider" },
    });
  });

  it("refuses without an org", async () => {
    await expect(listCatalogLLMProviders({} as never)).rejects.toThrow(
      "Missing required parameter: orgName",
    );
    expect(http.httpGET).not.toHaveBeenCalled();
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpGET.mockResolvedValue(json({ code: "FORBIDDEN" }, 403));
    await expect(listCatalogLLMProviders({ orgName: "o" })).rejects.toEqual({ code: "FORBIDDEN" });
  });
});

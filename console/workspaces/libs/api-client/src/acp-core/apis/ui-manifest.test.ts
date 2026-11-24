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

import { getUIManifest } from "./ui-manifest";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

afterEach(() => vi.resetAllMocks());

describe("getUIManifest", () => {
  it("reads the manifest with the caller's token", async () => {
    http.httpGET.mockResolvedValue(json({ modules: [{ id: "ai-gateway" }] }));
    await expect(getUIManifest(async () => "tok")).resolves.toEqual({
      modules: [{ id: "ai-gateway" }],
    });
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/ui-manifest", { token: "tok" });
  });

  it("reads it without a token, and throws a non-OK body", async () => {
    http.httpGET.mockResolvedValue(json({ code: "UNAUTHORIZED" }, 401));
    await expect(getUIManifest()).rejects.toEqual({ code: "UNAUTHORIZED" });
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/ui-manifest", { token: undefined });
  });
});

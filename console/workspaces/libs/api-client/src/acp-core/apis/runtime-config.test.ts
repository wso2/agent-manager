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
import type * as Types from "@agent-management-platform/types";
import type * as Utils from "../../shared/utils";

const http = vi.hoisted(() => ({ httpGET: vi.fn() }));
const config = vi.hoisted(() => ({
  apiBaseUrl: "https://api.test/",
  configDiscoveryBaseUrl: undefined as string | undefined,
}));
vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof Utils>()),
  ...http,
}));
vi.mock("@agent-management-platform/types", async (importOriginal) => ({
  ...(await importOriginal<typeof Types>()),
  globalConfig: config,
}));

import { getRuntimeConfig } from "./runtime-config";

const json = (body: unknown) => new Response(JSON.stringify(body));

afterEach(() => {
  vi.resetAllMocks();
  config.configDiscoveryBaseUrl = undefined;
});

describe("getRuntimeConfig", () => {
  it("reads the config from the API host, bounded by a timeout", async () => {
    http.httpGET.mockResolvedValue(json({ eventsEnabled: true }));

    await expect(getRuntimeConfig()).resolves.toEqual({ eventsEnabled: true });

    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/config", {
      baseUrl: "https://api.test",
      timeoutMs: 5000,
    });
  });

  it("prefers the public discovery host when one is configured", async () => {
    config.configDiscoveryBaseUrl = "  https://cfg.test//  ";
    http.httpGET.mockResolvedValue(json({}));
    await getRuntimeConfig();
    expect(http.httpGET.mock.calls[0][1].baseUrl).toBe("https://cfg.test");
  });

  it("falls back to the API host when the discovery host is blank", async () => {
    config.configDiscoveryBaseUrl = "   ";
    http.httpGET.mockResolvedValue(json({}));
    await getRuntimeConfig();
    expect(http.httpGET.mock.calls[0][1].baseUrl).toBe("https://api.test");
  });

  it("lets a failure through so the app can show an error", async () => {
    http.httpGET.mockRejectedValue(Object.assign(new Error("aborted"), { name: "AbortError" }));
    await expect(getRuntimeConfig()).rejects.toMatchObject({ name: "AbortError" });
  });
});

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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { backendFetch } from "@agent-management-platform/auth";
import { globalConfig } from "@agent-management-platform/types";
import { httpPOSTForm, isRequestTooLargeError } from "./utils";

vi.mock("@agent-management-platform/auth", () => ({
  backendFetch: vi.fn(),
  fetchWithBackendToken: vi.fn(),
}));

const config = globalConfig as unknown as Record<string, unknown>;
const saved = { apiBaseUrl: config.apiBaseUrl, maxRequestBodyBytes: config.maxRequestBodyBytes };

const form = (bytes: number) => {
  const f = new FormData();
  f.append("file", new File(["x".repeat(bytes)], "agents.csv"), "agents.csv");
  return f;
};

beforeEach(() => {
  vi.useFakeTimers();
  config.apiBaseUrl = "https://amp.example.com";
  vi.mocked(backendFetch).mockReset().mockResolvedValue(new Response("{}", { status: 200 }));
});

afterEach(() => {
  vi.useRealTimers();
  Object.assign(config, saved);
});

describe("httpPOSTForm", () => {
  it("sends the form with the token and no Content-Type, so the browser sets the boundary", async () => {
    config.maxRequestBodyBytes = 0;
    const body = form(10);
    const sent = httpPOSTForm("/api/v1/upload", body, { token: "jwt" });
    await vi.runAllTimersAsync();
    await sent;
    const [url, init] = vi.mocked(backendFetch).mock.calls[0];
    expect(url).toBe("https://amp.example.com/api/v1/upload");
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe(body);
    const headers = new Headers(init?.headers);
    expect(headers.get("Authorization")).toBe("Bearer jwt");
    expect(headers.has("Content-Type")).toBe(false);
  });

  it("refuses a form larger than MAX_REQUEST_BODY_BYTES without sending it", async () => {
    config.maxRequestBodyBytes = 100;
    const err = await httpPOSTForm("/api/v1/upload", form(101), {}).catch((e: unknown) => e);
    expect(isRequestTooLargeError(err)).toBe(true);
    expect((err as { status?: number }).status).toBe(413);
    expect(backendFetch).not.toHaveBeenCalled();
  });
});

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

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type * as Types from "@agent-management-platform/types";

const auth = vi.hoisted(() => ({
  backendFetch: vi.fn(),
  fetchWithBackendToken: vi.fn(),
}));
const limits = vi.hoisted(() => ({ maxBody: 0 }));

vi.mock("@agent-management-platform/auth", () => auth);
vi.mock("@agent-management-platform/types", async (importOriginal) => ({
  ...(await importOriginal<typeof Types>()),
  globalConfig: { apiBaseUrl: "https://api.test" },
  getMaxRequestBodyBytes: () => limits.maxBody,
}));

import {
  encodeRequired,
  GATEWAY_BASE,
  httpDELETE,
  httpGET,
  httpGETObserver,
  httpPATCH,
  httpPOST,
  httpPUT,
  isObserverConfigured,
  isRequestTooLargeError,
  moduleBase,
  REQUEST_TOO_LARGE_CODE,
  serializeRequestBody,
  setObserverBaseUrl,
  sleep,
} from "./utils";

const json = (body: unknown, status = 200) =>
  new Response(body === undefined ? null : JSON.stringify(body), { status });

/** Runs a helper call to completion, skipping its built-in settle delay. */
async function settle<T>(promise: Promise<T>): Promise<T> {
  const outcome = promise.then(
    (value) => ({ value }),
    (error: unknown) => ({ error }),
  );
  await vi.runAllTimersAsync();
  const result = (await outcome) as { value?: T; error?: unknown };
  if ("error" in result) throw result.error;
  return result.value as T;
}

beforeEach(() => {
  vi.useFakeTimers();
  limits.maxBody = 0;
});

afterEach(() => {
  vi.useRealTimers();
  vi.resetAllMocks();
  setObserverBaseUrl(undefined);
});

describe("path helpers", () => {
  it("serves modules beside the core API", () => {
    expect(moduleBase("agent-runtime")).toBe("/m/agent-runtime");
    expect(GATEWAY_BASE).toBe("/m/ai-gateway");
  });

  it("encodes a required path parameter and refuses a missing one", () => {
    expect(encodeRequired("a b/c", "orgName")).toBe("a%20b%2Fc");
    expect(() => encodeRequired("", "orgName")).toThrow("Missing required parameter: orgName");
    expect(() => encodeRequired(undefined, "gatewayId")).toThrow(/gatewayId/);
  });

  it("sleeps for the given time", async () => {
    const done = vi.fn();
    void sleep(500).then(done);
    await vi.advanceTimersByTimeAsync(499);
    expect(done).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(done).toHaveBeenCalled();
  });
});

describe("httpGET", () => {
  it("sends a GET with the query string and the bearer token", async () => {
    auth.backendFetch.mockResolvedValue(json({ ok: 1 }));

    const res = await settle(
      httpGET("/api/v1/orgs", { searchParams: { limit: "5", q: "a b" }, token: "tok" }),
    );

    expect(await res.json()).toEqual({ ok: 1 });
    const [url, init] = auth.backendFetch.mock.calls[0];
    expect(url).toBe("https://api.test/api/v1/orgs?limit=5&q=a+b");
    expect(init.method).toBe("GET");
    expect(init.headers).toEqual({
      "Content-Type": "application/json",
      Authorization: "Bearer tok",
    });
    expect(init.signal).toBeUndefined();
  });

  it("repeats array-form parameters and omits Authorization without a token", async () => {
    auth.backendFetch.mockResolvedValue(json({}));

    await settle(httpGET("/x", { searchParams: [["label", "a"], ["label", "b"]] }));

    const [url, init] = auth.backendFetch.mock.calls[0];
    expect(url).toBe("https://api.test/x?label=a&label=b");
    expect(init.headers).toEqual({ "Content-Type": "application/json" });
  });

  it("targets an overriding base URL and arms an abort timeout", async () => {
    auth.backendFetch.mockImplementation(
      (_url: string, init: RequestInit) =>
        new Promise((_resolve, reject) => {
          init.signal?.addEventListener("abort", () =>
            reject(Object.assign(new Error("aborted"), { name: "AbortError" })),
          );
        }),
    );

    const pending = settle(httpGET("/api/v1/config", { baseUrl: "https://cfg.test", timeoutMs: 50 }));

    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
    expect(auth.backendFetch.mock.calls[0][0]).toBe("https://cfg.test/api/v1/config?");
  });

  it("throws an error carrying the status on a non-OK response", async () => {
    auth.backendFetch.mockResolvedValue(json({ message: "nope" }, 403));

    await expect(settle(httpGET("/x", {}))).rejects.toMatchObject({
      message: "HTTP error! status: 403",
      status: 403,
    });
  });
});

describe("observer requests", () => {
  it("refuses to call an observer that is not configured", async () => {
    expect(isObserverConfigured()).toBe(false);
    await expect(httpGETObserver("/api/v1/traces", {})).rejects.toThrow(/Observer is not configured/);
    expect(auth.fetchWithBackendToken).not.toHaveBeenCalled();
  });

  it("calls the observer base URL without a doubled slash", async () => {
    setObserverBaseUrl("  https://obs.test//  ");
    expect(isObserverConfigured()).toBe(true);
    auth.fetchWithBackendToken.mockResolvedValue(json({ traces: [] }));

    const res = await settle(
      httpGETObserver("/api/v1/traces", { searchParams: { limit: "1" }, token: "tok" }),
    );

    expect(await res.json()).toEqual({ traces: [] });
    const [url, init] = auth.fetchWithBackendToken.mock.calls[0];
    expect(url).toBe("https://obs.test/api/v1/traces?limit=1");
    expect(init.headers.Authorization).toBe("Bearer tok");
  });

  it("sends no Authorization without a token and surfaces a non-OK status", async () => {
    setObserverBaseUrl("https://obs.test");
    auth.fetchWithBackendToken.mockResolvedValue(json({}, 502));

    await expect(settle(httpGETObserver("/t", {}))).rejects.toMatchObject({ status: 502 });
    expect(auth.fetchWithBackendToken.mock.calls[0][1].headers).toEqual({
      "Content-Type": "application/json",
    });
  });

  it("treats a blank base URL as unconfigured", () => {
    setObserverBaseUrl("   ");
    expect(isObserverConfigured()).toBe(false);
  });
});

describe.each([
  ["POST", (token?: string) => httpPOST("/w", { a: 1 }, { searchParams: { dry: "1" }, token })],
  ["PUT", (token?: string) => httpPUT("/w", { a: 1 }, { searchParams: { dry: "1" }, token })],
  ["PATCH", (token?: string) => httpPATCH("/w", { a: 1 }, { searchParams: { dry: "1" }, token })],
] as const)("http%s", (method, send) => {
  it("sends the JSON body with the bearer token", async () => {
    auth.backendFetch.mockResolvedValue(json({ id: "1" }, 201));

    const res = await settle(send("tok"));

    expect(res.status).toBe(201);
    const [url, init] = auth.backendFetch.mock.calls[0];
    expect(url).toBe("https://api.test/w?dry=1");
    expect(init.method).toBe(method);
    expect(init.body).toBe('{"a":1}');
    expect(init.headers.Authorization).toBe("Bearer tok");
  });

  it("omits Authorization without a token", async () => {
    auth.backendFetch.mockResolvedValue(json({}));
    await settle(send());
    expect(auth.backendFetch.mock.calls[0][1].headers).toEqual({ "Content-Type": "application/json" });
  });

  it("throws the server's message and body on a non-OK response", async () => {
    const body = { code: "CONFLICT", message: "Name already taken", reason: "dup" };
    auth.backendFetch.mockResolvedValue(json(body, 409));

    await expect(settle(send("tok"))).rejects.toMatchObject({
      message: "Name already taken",
      status: 409,
      body,
    });
  });

  it("falls back to the status when the error body has no message", async () => {
    auth.backendFetch.mockResolvedValue(new Response("<html>", { status: 500 }));
    await expect(settle(send())).rejects.toMatchObject({
      message: "HTTP error! status: 500",
      status: 500,
      body: undefined,
    });
  });
});

describe("httpDELETE", () => {
  it("sends a DELETE with the token and no body", async () => {
    auth.backendFetch.mockResolvedValue(new Response(null, { status: 204 }));

    const res = await settle(httpDELETE("/w/1", { token: "tok" }));

    expect(res.status).toBe(204);
    const [url, init] = auth.backendFetch.mock.calls[0];
    expect(url).toBe("https://api.test/w/1?");
    expect(init).toMatchObject({ method: "DELETE", headers: { Authorization: "Bearer tok" } });
    expect(init.body).toBeUndefined();
  });

  it("sends no Authorization without a token and throws on non-OK", async () => {
    auth.backendFetch.mockResolvedValue(json({ message: 123 }, 404));
    await expect(settle(httpDELETE("/w/1", {}))).rejects.toMatchObject({
      message: "HTTP error! status: 404",
      status: 404,
    });
    expect(auth.backendFetch.mock.calls[0][1].headers).toEqual({ "Content-Type": "application/json" });
  });
});

describe("httpPOST abort", () => {
  it("reports an abort during the settle delay as an abort, not the response's error", async () => {
    auth.backendFetch.mockResolvedValue(json({ message: "boom" }, 500));
    const controller = new AbortController();

    const pending = settle(httpPOST("/w", {}, { signal: controller.signal }));
    controller.abort();

    await expect(pending).rejects.toMatchObject({ name: "AbortError" });
    expect(auth.backendFetch.mock.calls[0][1].signal).toBe(controller.signal);
  });
});

describe("serializeRequestBody", () => {
  it("serialises any body when the deployment sets no limit", () => {
    limits.maxBody = 0;
    expect(serializeRequestBody({ text: "x".repeat(10_000) })).toHaveLength(10_011);
  });

  it("serialises a body within the limit", () => {
    limits.maxBody = 100;
    expect(serializeRequestBody({ a: "é" })).toBe('{"a":"é"}');
  });

  it("refuses a body over the limit with a 413 the error handlers recognise", () => {
    limits.maxBody = 10;
    let thrown: unknown;
    try {
      serializeRequestBody({ text: "too long for ten bytes" });
    } catch (error) {
      thrown = error;
    }
    expect(thrown).toMatchObject({ status: 413, code: REQUEST_TOO_LARGE_CODE });
    expect((thrown as Error).message).toMatch(/^Request is too large \(.+ of .+\)\. Shorten/);
    expect(isRequestTooLargeError(thrown)).toBe(true);
  });

  it("does not send a write whose body is over the limit", async () => {
    limits.maxBody = 5;
    await expect(httpPUT("/w", { long: "value" }, {})).rejects.toMatchObject({ status: 413 });
    expect(auth.backendFetch).not.toHaveBeenCalled();
  });

  it("tells a size refusal apart from other errors", () => {
    expect(isRequestTooLargeError(new Error("x"))).toBe(false);
    expect(isRequestTooLargeError({ code: REQUEST_TOO_LARGE_CODE })).toBe(false);
  });
});

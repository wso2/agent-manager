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

// Test-only table runner for API functions built on the shared http helpers.
// The calling test file must vi.mock "shared/utils" so httpGET/POST/PUT/DELETE
// are mocks, and pass them in here.

import { beforeEach, describe, expect, it, type Mock } from "vitest";

export type HttpMethod = "GET" | "POST" | "PUT" | "DELETE";

export type HttpMocks = Record<HttpMethod, Mock>;

export interface HttpCase {
  name: string;
  /** Calls the API function with a getToken that resolves to "jwt". */
  call: () => Promise<unknown>;
  method: HttpMethod;
  url: string;
  /** Body sent with POST/PUT; defaults to {}. */
  body?: unknown;
  searchParams?: unknown;
  /** Whether the function resolves to the response body (default) or to nothing. */
  returnsBody?: boolean;
  /** Body served on success; defaults to RESPONSE. */
  response?: unknown;
  /** What the function resolves to; defaults to the served body. */
  expected?: unknown;
  /** False for functions that leave non-OK handling to the http helper. */
  checksOk?: boolean;
}

export const RESPONSE = { id: "r-1", list: [] };

export const jsonResponse = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), { status });

export const getToken = async (): Promise<string> => "jwt";

function expectRequest(mocks: HttpMocks, c: HttpCase): void {
  const others = (Object.keys(mocks) as HttpMethod[]).filter((m) => m !== c.method);
  others.forEach((m) => expect(mocks[m], `${m} should not be called`).not.toHaveBeenCalled());
  const fn = mocks[c.method];
  expect(fn).toHaveBeenCalledTimes(1);
  const args = fn.mock.calls[0];
  expect(args[0]).toBe(c.url);
  const withBody = c.method === "POST" || c.method === "PUT";
  if (withBody) expect(args[1]).toEqual(c.body ?? {});
  const options = args[withBody ? 2 : 1] as { token?: string; searchParams?: unknown };
  expect(options.token).toBe("jwt");
  expect(options.searchParams).toEqual(c.searchParams);
}

/** Registers request-shape, response and error tests for each case. */
export function describeHttpCases(title: string, mocks: HttpMocks, cases: HttpCase[]): void {
  describe(title, () => {
    beforeEach(() => {
      mocks.GET.mockReset().mockImplementation(async () => jsonResponse(RESPONSE));
      mocks.POST.mockReset().mockImplementation(async () => jsonResponse(RESPONSE, 201));
      mocks.PUT.mockReset().mockImplementation(async () => jsonResponse(RESPONSE));
      mocks.DELETE.mockReset().mockImplementation(async () => new Response(null, { status: 204 }));
    });

    it.each(cases.map((c) => [c.name, c] as const))("%s sends the expected request", async (_n, c) => {
      if (c.response !== undefined) {
        mocks[c.method].mockImplementation(async () => jsonResponse(c.response));
      }
      const result = await c.call();
      expectRequest(mocks, c);
      if (c.returnsBody === false) expect(result).toBeUndefined();
      else expect(result).toEqual(c.expected ?? c.response ?? RESPONSE);
    });

    const checked = cases.filter((c) => c.checksOk !== false);
    if (checked.length) {
      it.each(checked.map((c) => [c.name, c] as const))("%s rejects with the error body on a non-OK response", async (_n, c) => {
        mocks[c.method].mockImplementation(async () => jsonResponse({ message: "boom" }, 409));
        await expect(c.call()).rejects.toEqual({ message: "boom" });
      });
    }

    const unchecked = cases.filter((c) => c.checksOk === false);
    if (unchecked.length) {
      it.each(unchecked.map((c) => [c.name, c] as const))("%s propagates the http helper's error", async (_n, c) => {
        const error = Object.assign(new Error("HTTP error! status: 500"), { status: 500 });
        mocks[c.method].mockRejectedValue(error);
        await expect(c.call()).rejects.toBe(error);
      });
    }
  });
}

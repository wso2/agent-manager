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

import { act } from "react";
import { createRoot } from "react-dom/client";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { describe, expect, it } from "vitest";
import { ClientProvider } from "./ClientProvider";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function providedClient(): QueryClient {
  let client: QueryClient | undefined;
  function Probe() {
    client = useQueryClient();
    return <span>child</span>;
  }
  const container = document.createElement("div");
  const root = createRoot(container);
  act(() => root.render(<ClientProvider><Probe /></ClientProvider>));
  expect(container.textContent).toBe("child");
  act(() => root.unmount());
  return client!;
}

type Retry = (failureCount: number, error: unknown) => boolean;

describe("ClientProvider", () => {
  it("gives its children one shared client, refetching on focus with a short stale time", () => {
    const client = providedClient();
    expect(providedClient()).toBe(client);
    const queries = client.getDefaultOptions().queries!;
    expect(queries.refetchOnWindowFocus).toBe(true);
    expect(queries.staleTime).toBe(8000);
  });

  it.each([400, 401, 403, 404, 409, 422])("does not retry a %i, which will not succeed later", (status) => {
    const retry = providedClient().getDefaultOptions().queries!.retry as Retry;
    expect(retry(0, { status })).toBe(false);
  });

  it.each([
    ["a 408", { status: 408 }],
    ["a 5xx", { status: 503 }],
    ["a network failure", new TypeError("Failed to fetch")],
    ["an unknown rejection", undefined],
  ])("retries %s up to three times", (_name, error) => {
    const retry = providedClient().getDefaultOptions().queries!.retry as Retry;
    expect([0, 1, 2].map((n) => retry(n, error))).toEqual([true, true, true]);
    expect(retry(3, error)).toBe(false);
  });
});

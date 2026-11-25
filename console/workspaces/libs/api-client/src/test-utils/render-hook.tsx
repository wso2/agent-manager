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

// Test-only: a minimal renderHook for this package, which has no
// @testing-library dependency.

import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

/** A QueryClient that fails fast, as a test wants. */
export function testQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
}

export interface RenderedHook<T> {
  result: { current: T };
  client: QueryClient;
  rerender: () => Promise<void>;
  unmount: () => void;
}

/** Renders a hook inside a QueryClientProvider (and an optional extra wrapper). */
export async function renderHook<T>(
  useHook: () => T,
  options: { client?: QueryClient; wrapper?: (children: ReactNode) => ReactNode } = {},
): Promise<RenderedHook<T>> {
  const client = options.client ?? testQueryClient();
  const wrap = options.wrapper ?? ((children: ReactNode) => children);
  const result = {} as { current: T };
  function Probe() {
    const value = useHook();
    // A query result only re-renders for the fields read while rendering.
    // Read them all, so a test can wait on any of them.
    if (value && typeof value === "object") {
      Object.keys(value).forEach((key) => void (value as Record<string, unknown>)[key]);
    }
    result.current = value;
    return null;
  }
  const root = createRoot(document.createElement("div"));
  const render = () =>
    root.render(<QueryClientProvider client={client}>{wrap(<Probe />)}</QueryClientProvider>);
  await act(async () => render());
  return {
    result,
    client,
    rerender: () => act(async () => render()),
    unmount: () => act(() => root.unmount()),
  };
}

/** Lets React and TanStack Query settle until `check` holds, or fails. */
export async function waitFor(check: () => boolean): Promise<void> {
  const deadline = Date.now() + 2000;
  while (!check() && Date.now() < deadline) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 5));
    });
  }
  if (!check()) {
    throw new Error("waitFor: condition never held");
  }
}

/** Seeds `keys` in a client, so a test can see which ones a mutation invalidates. */
export function seed(client: QueryClient, keys: readonly (readonly unknown[])[]): void {
  keys.forEach((key) => client.setQueryData(key, { seeded: true }));
}

/** The seeded keys a mutation marked stale. */
export function invalidatedKeys(
  client: QueryClient,
  keys: readonly (readonly unknown[])[],
): unknown[][] {
  return keys.filter((key) => client.getQueryState(key)?.isInvalidated).map((key) => [...key]);
}

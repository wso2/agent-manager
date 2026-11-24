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

// Test-only helpers for rendering query/mutation hooks against a real QueryClient.

import { act, createElement } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, type QueryKey } from "@tanstack/react-query";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const mounted: Array<() => void> = [];

/** Unmounts every hook rendered since the last call; run it in afterEach. */
export function cleanupHooks(): void {
  while (mounted.length) mounted.pop()?.();
}

export function newQueryClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

/** Renders a hook inside a QueryClientProvider and returns its latest result. */
export async function renderHook<T>(useHook: () => T, qc: QueryClient = newQueryClient()) {
  const result = {} as { current: T };
  const Probe = () => {
    result.current = useHook();
    return null;
  };
  const root = createRoot(document.createElement("div"));
  await act(async () =>
    root.render(createElement(QueryClientProvider, { client: qc }, createElement(Probe))),
  );
  mounted.push(() => act(() => root.unmount()));
  return { result, qc };
}

/** Lets pending query functions and their state updates settle. */
export async function flush(ms = 20): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });
}

/**
 * Runs a mutation to completion with the given cache entries seeded, and
 * reports which seeded entries it invalidated (sorted by name).
 */
export async function runMutation(
  useHook: () => { mutateAsync: (variables: never) => Promise<unknown> },
  variables: unknown,
  seeded: Record<string, QueryKey> = {},
) {
  const qc = newQueryClient();
  Object.values(seeded).forEach((key) => qc.setQueryData(key, {}));
  const { result } = await renderHook(useHook, qc);
  let value: unknown;
  let error: unknown;
  await act(async () => {
    try {
      value = await result.current.mutateAsync(variables as never);
    } catch (e) {
      error = e;
    }
  });
  const invalidated = Object.entries(seeded)
    .filter(([, key]) => qc.getQueryState(key)?.isInvalidated)
    .map(([name]) => name)
    .sort();
  return { value, error, invalidated, qc };
}

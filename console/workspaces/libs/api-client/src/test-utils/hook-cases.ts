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

// Test-only table runners for query and mutation hooks. The calling test file
// mocks the API module, auth, the snackbar and the UI-module registry.

import { afterEach, describe, expect, it, type Mock } from "vitest";
import type { QueryKey, UseQueryResult } from "@tanstack/react-query";
import { cleanupHooks, flush, renderHook, runMutation } from "./query-hooks";

export const DATA = { id: "d-1", list: [{ id: "x" }] };

export interface QueryHookCase {
  name: string;
  /** Renders the hook with complete params. */
  use: () => unknown;
  api: Mock;
  /** Arguments the API function receives before getToken. */
  args: unknown[];
  key: QueryKey;
  /** What the API resolves to (default DATA) and what the hook exposes (default the same). */
  data?: unknown;
  expected?: unknown;
  /** Renders of the hook that must not fetch: missing params, or disabled by the caller. */
  idle?: Array<() => unknown>;
  /** False when the hook does not check the UI module registry. */
  gated?: boolean;
}

export interface QueryHookOptions {
  /** Installs or removes the UI module the hooks are gated on. */
  setModulePresent?: (present: boolean) => void;
}

const asQuery = (value: unknown) => value as UseQueryResult;

export function describeQueryHooks(
  title: string,
  cases: QueryHookCase[],
  options: QueryHookOptions = {},
): void {
  const rows = cases.map((c) => [c.name, c] as const);
  describe(title, () => {
    afterEach(() => {
      cleanupHooks();
      options.setModulePresent?.(true);
    });

    it.each(rows)("%s fetches through the API and caches under its key", async (_n, c) => {
      c.api.mockReset().mockResolvedValue(c.data ?? DATA);
      const { qc } = await renderHook(c.use);
      await flush();
      expect(c.api).toHaveBeenCalledWith(...c.args, expect.any(Function));
      const token = c.api.mock.calls[0][c.args.length] as () => Promise<string>;
      await expect(token()).resolves.toBe("t");
      const expected = c.expected ?? c.data ?? DATA;
      expect(qc.getQueryState(c.key)?.status).toBe("success");
      expect(qc.getQueryData(c.key)).toEqual(expected);
    });

    it.each(rows)("%s surfaces an API failure as a query error", async (_n, c) => {
      c.api.mockReset().mockRejectedValue(Object.assign(new Error("boom"), { status: 500 }));
      const { result } = await renderHook(c.use);
      await flush();
      expect(asQuery(result.current).isError).toBe(true);
      expect((asQuery(result.current).error as Error).message).toBe("boom");
    });

    const idleRows = cases.flatMap((c) => (c.idle ?? []).map((use, i) => [`${c.name} #${i + 1}`, c, use] as const));
    if (idleRows.length) {
      it.each(idleRows)("%s stays idle while its inputs are incomplete", async (_n, c, use) => {
        c.api.mockReset().mockResolvedValue(DATA);
        const { result } = await renderHook(use);
        await flush();
        expect(c.api).not.toHaveBeenCalled();
        expect(asQuery(result.current).fetchStatus).toBe("idle");
        expect(asQuery(result.current).data).toBeUndefined();
      });
    }

    const setModulePresent = options.setModulePresent;
    const gatedRows = rows.filter(([, c]) => c.gated !== false);
    if (setModulePresent && gatedRows.length) {
      it.each(gatedRows)("%s stays idle when its UI module is not installed", async (_n, c) => {
        setModulePresent(false);
        c.api.mockReset().mockResolvedValue(DATA);
        const { result } = await renderHook(c.use);
        await flush();
        expect(c.api).not.toHaveBeenCalled();
        expect(asQuery(result.current).fetchStatus).toBe("idle");
      });
    }
  });
}

export interface MutationHookCase {
  name: string;
  use: () => { mutateAsync: (variables: never) => Promise<unknown> };
  api: Mock;
  variables: unknown;
  /** Arguments the API function receives before getToken. */
  args: unknown[];
  /** Arguments the API function receives after getToken. */
  after?: unknown[];
  /** Names of the seeded cache entries the mutation must invalidate. */
  invalidates: string[];
  /** What the API resolves to (default DATA) and the mutation resolves to (default the same). */
  response?: unknown;
  expected?: unknown;
  /** The mutation resolves to nothing whatever the API returns. */
  voidResult?: boolean;
}

export function describeMutationHooks(
  title: string,
  seeded: Record<string, QueryKey>,
  cases: MutationHookCase[],
): void {
  const rows = cases.map((c) => [c.name, c] as const);
  describe(title, () => {
    afterEach(() => cleanupHooks());

    it.each(rows)("%s calls the API and invalidates exactly the affected views", async (_n, c) => {
      c.api.mockReset().mockResolvedValue(c.response ?? DATA);
      const { value, error, invalidated } = await runMutation(c.use, c.variables, seeded);
      expect(error).toBeUndefined();
      expect(c.api).toHaveBeenCalledWith(...c.args, expect.any(Function), ...(c.after ?? []));
      if (c.voidResult) expect(value).toBeUndefined();
      else expect(value).toEqual(c.expected ?? c.response ?? DATA);
      expect(invalidated).toEqual([...c.invalidates].sort());
    });

    it.each(rows)("%s rejects and refreshes nothing when the API fails", async (_n, c) => {
      const failure = Object.assign(new Error("conflict"), { status: 409 });
      c.api.mockReset().mockRejectedValue(failure);
      const { error, invalidated } = await runMutation(c.use, c.variables, seeded);
      expect(error).toBe(failure);
      expect(invalidated).toEqual([]);
    });
  });
}

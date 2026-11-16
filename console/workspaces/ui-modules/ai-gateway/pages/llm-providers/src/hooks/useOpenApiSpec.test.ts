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

import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useOpenApiSpec } from "./useOpenApiSpec";

const SPEC = "openapi: 3.0.0\npaths: {}\n";

describe("useOpenApiSpec", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("does not report a spec fetched from the template URL as modified", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, text: () => Promise.resolve(SPEC) }),
    );

    // Unique URL so the module-level cache from other tests doesn't apply.
    const { result } = renderHook(() =>
      useOpenApiSpec("https://example.test/fetched.yaml", ""),
    );

    await waitFor(() => expect(result.current.text).toBe(SPEC));
    expect(result.current.isModified).toBe(false);
  });

  it("tracks an imported spec until it is saved or reset", () => {
    const { result } = renderHook(() => useOpenApiSpec(undefined, SPEC));
    expect(result.current.isModified).toBe(false);

    act(() => result.current.setText("openapi: 3.1.0\n"));
    expect(result.current.isModified).toBe(true);

    act(() => result.current.reset());
    expect(result.current.text).toBe(SPEC);
    expect(result.current.isModified).toBe(false);

    act(() => result.current.setText("openapi: 3.1.0\n"));
    act(() => result.current.markSaved());
    expect(result.current.isModified).toBe(false);
  });
});

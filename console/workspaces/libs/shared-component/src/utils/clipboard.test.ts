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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { copyToClipboard, useCopyOnSuccess } from "./clipboard";

const originalClipboard = Object.getOwnPropertyDescriptor(navigator, "clipboard");

function setClipboard(value: unknown) {
  Object.defineProperty(navigator, "clipboard", { value, configurable: true });
}

afterEach(() => {
  if (originalClipboard) Object.defineProperty(navigator, "clipboard", originalClipboard);
  else delete (navigator as unknown as Record<string, unknown>).clipboard;
});

describe("copyToClipboard", () => {
  it("writes the text and reports success", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    setClipboard({ writeText });
    await expect(copyToClipboard("hello")).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith("hello");
  });

  it("reports failure when the Clipboard API is unavailable", async () => {
    setClipboard(undefined);
    await expect(copyToClipboard("hello")).resolves.toBe(false);
  });

  it("reports failure when the write is rejected", async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error("denied")) });
    await expect(copyToClipboard("hello")).resolves.toBe(false);
  });
});

describe("useCopyOnSuccess", () => {
  it("notifies only after a successful copy", async () => {
    setClipboard({ writeText: vi.fn().mockResolvedValue(undefined) });
    const onCopy = vi.fn();
    const { result } = renderHook(() => useCopyOnSuccess(onCopy));
    await act(async () => {
      result.current("token", "API key");
    });
    expect(onCopy).toHaveBeenCalledWith("token", "API key");
  });

  it("stays silent when the copy fails", async () => {
    setClipboard({ writeText: vi.fn().mockRejectedValue(new Error("denied")) });
    const onCopy = vi.fn();
    const { result } = renderHook(() => useCopyOnSuccess(onCopy));
    await act(async () => {
      result.current("token", "API key");
    });
    expect(onCopy).not.toHaveBeenCalled();
  });
});

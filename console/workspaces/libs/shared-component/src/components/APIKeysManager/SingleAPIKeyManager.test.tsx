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

import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

// MUI transitions and first renders are slow on a busy machine.
vi.setConfig({ testTimeout: 30000 });
import type { APIKeyInfo } from "@agent-management-platform/types";
import { SingleAPIKeyManager } from "./SingleAPIKeyManager";

const existing = {
  name: "cfg-key",
  displayName: "Config key",
  maskedApiKey: "sk-****1234",
  status: "active",
  expiresAt: "2031-05-06T00:00:00Z",
} as APIKeyInfo;

function setup(props: Partial<React.ComponentProps<typeof SingleAPIKeyManager>> = {}) {
  const handlers = {
    onGenerate: vi.fn<() => Promise<string | undefined>>(),
    onDelete: vi.fn(),
    onRegenerate: vi.fn<() => Promise<string | undefined>>(),
  };
  const view = (p: Partial<React.ComponentProps<typeof SingleAPIKeyManager>>) => (
    <SingleAPIKeyManager
      apiKey={undefined}
      isLoading={false}
      isGenerating={false}
      {...handlers}
      {...p}
    />
  );
  const utils = render(view(props));
  return { ...handlers, ...utils, rerenderWith: (p: typeof props) => utils.rerender(view(p)) };
}

describe("SingleAPIKeyManager", () => {
  it("shows the empty state and generates a key that is shown once", async () => {
    const { onGenerate } = setup({ description: "Keys for this config" });
    onGenerate.mockResolvedValue("sk-new");
    expect(screen.getByText("API Keys")).toBeInTheDocument();
    expect(screen.getByText("Keys for this config")).toBeInTheDocument();
    expect(screen.getByText("No API key generated yet")).toBeInTheDocument();
    expect(
      screen.getByText("Generate an API key to authenticate requests through this configuration."),
    ).toBeInTheDocument();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    });
    expect(screen.getByDisplayValue("sk-new")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByDisplayValue("sk-new")).not.toBeInTheDocument();
  });

  it("copies the one-time key to the clipboard", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });
    const { onGenerate } = setup();
    onGenerate.mockResolvedValue("sk-copy");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Copy API key" }));
    });
    expect(writeText).toHaveBeenCalledWith("sk-copy");
  });

  it("swallows a failed generate without showing a key", async () => {
    const { onGenerate } = setup();
    onGenerate.mockRejectedValue(new Error("nope"));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    });
    expect(screen.queryByText(/You will only see this key once/)).not.toBeInTheDocument();
  });

  it("shows the existing key with its expiry and status", () => {
    setup({ apiKey: existing });
    expect(screen.getByText("Config key")).toBeInTheDocument();
    expect(
      screen.getByText(`sk-****1234 · Expires ${new Date("2031-05-06T00:00:00Z").toLocaleDateString()}`),
    ).toBeInTheDocument();
    expect(screen.getByText("active")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Generate" })).not.toBeInTheDocument();
  });

  it("deletes only after confirming", async () => {
    const { onDelete } = setup({ apiKey: existing });
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(screen.getByText("Delete API key?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onDelete).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByText("Delete API key?")).not.toBeInTheDocument(), {
      timeout: 10000,
    });
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Delete" }).slice(-1)[0]);
    expect(onDelete).toHaveBeenCalled();
  });

  it("regenerates after confirming and shows the new key", async () => {
    const { onRegenerate } = setup({ apiKey: { ...existing, displayName: "", status: "revoked" } as APIKeyInfo });
    onRegenerate.mockResolvedValue("sk-regen");
    expect(screen.getByText("cfg-key")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Regenerate" }));
    expect(screen.getByText("Regenerate API key?")).toBeInTheDocument();
    await act(async () => {
      fireEvent.click(screen.getAllByRole("button", { name: "Regenerate" }).slice(-1)[0]);
    });
    expect(onRegenerate).toHaveBeenCalled();
    expect(screen.getByDisplayValue("sk-regen")).toBeInTheDocument();
  });

  it("keeps the regenerate dialog open when it fails", async () => {
    const { onRegenerate } = setup({ apiKey: existing });
    onRegenerate.mockRejectedValue(new Error("fail"));
    fireEvent.click(screen.getByRole("button", { name: "Regenerate" }));
    await act(async () => {
      fireEvent.click(screen.getAllByRole("button", { name: "Regenerate" }).slice(-1)[0]);
    });
    expect(screen.getByText("Regenerate API key?")).toBeInTheDocument();
  });

  it("resets the one-time key when the scope changes", async () => {
    const { onGenerate, rerenderWith } = setup({ scopeKey: "a" });
    onGenerate.mockResolvedValue("sk-a");
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    });
    expect(screen.getByDisplayValue("sk-a")).toBeInTheDocument();
    rerenderWith({ scopeKey: "b" });
    expect(screen.queryByDisplayValue("sk-a")).not.toBeInTheDocument();
  });

  it.each([
    [{ isLoading: true }, null],
    [{ disabledReason: "Enable API key auth first." }, "Enable API key auth first."],
    [{ isError: true }, "Failed to load the API key. Please refresh the page."],
  ])("hides generate for %j", (props, message) => {
    setup(props);
    expect(screen.queryByRole("button", { name: "Generate" })).not.toBeInTheDocument();
    if (message) expect(screen.getByText(message)).toBeInTheDocument();
  });

  it("is view-only when read-only", () => {
    const { unmount } = setup({ readOnly: true, apiKey: existing });
    expect(screen.queryByRole("button", { name: "Delete" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Regenerate" })).not.toBeInTheDocument();
    unmount();
    setup({ readOnly: true });
    expect(screen.queryByRole("button", { name: "Generate" })).not.toBeInTheDocument();
    expect(screen.queryByText(/Generate an API key to authenticate/)).not.toBeInTheDocument();
  });

  it("shows progress labels while busy", () => {
    const { rerenderWith } = setup({ isGenerating: true });
    expect(screen.getByRole("button", { name: "Generating…" })).toBeDisabled();
    rerenderWith({ apiKey: existing, isDeleting: true, isRegenerating: true });
    expect(screen.getByRole("button", { name: "Regenerate" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
  });
});

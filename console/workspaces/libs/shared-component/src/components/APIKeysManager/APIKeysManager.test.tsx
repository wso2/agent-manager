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
import { beforeEach, describe, expect, it, vi } from "vitest";

// MUI transitions and first renders are slow on a busy machine.
vi.setConfig({ testTimeout: 30000 });
import { addMonths, format } from "date-fns";
import type { APIKeyInfo } from "@agent-management-platform/types";

const track = vi.fn();
vi.mock("@agent-management-platform/api-client", () => ({
  useTrack: () => ({ track }),
  ConsoleAction: {
    SecretRevealed: "secret-revealed",
    ConfirmationCancelled: "confirmation-cancelled",
  },
}));

import { APIKeysManager, isApiKeyAuthEnabled } from "./APIKeysManager";
import { ConfirmationDialogProvider } from "../ConfirmationDialog/ConfirmationDialogProvider";

const keys: APIKeyInfo[] = [
  {
    name: "prod-key",
    displayName: "Prod key",
    maskedApiKey: "sk-****abcd",
    status: "active",
    expiresAt: "2030-01-02T03:04:05Z",
  } as APIKeyInfo,
  {
    name: "old-key",
    maskedApiKey: "sk-****zzzz",
    status: "revoked",
  } as APIKeyInfo,
];

function setup(props: Partial<React.ComponentProps<typeof APIKeysManager>> = {}) {
  const onCreate =
    vi.fn<(i: { displayName: string; expiresAt: string }) => Promise<string | undefined>>();
  const onRevoke = vi.fn();
  const utils = render(
    <ConfirmationDialogProvider>
      <APIKeysManager
        keys={keys}
        isLoading={false}
        isCreating={false}
        emptyDescription="Create a key for this agent."
        onCreate={onCreate}
        onRevoke={onRevoke}
        {...props}
      />
    </ConfirmationDialogProvider>,
  );
  return { onCreate, onRevoke, ...utils };
}

const nameInput = () => document.getElementById("displayName") as HTMLInputElement;

describe("isApiKeyAuthEnabled", () => {
  it.each([
    [undefined, false],
    [null, false],
    [{ apiKey: { enabled: true } }, true],
    [{ enabled: true, apiKey: { enabled: true } }, true],
    [{ enabled: false, apiKey: { enabled: true } }, false],
    [{ enabled: true, apiKey: { enabled: false } }, false],
  ])("%j -> %s", (security, expected) => {
    expect(isApiKeyAuthEnabled(security as never)).toBe(expected);
  });
});

describe("APIKeysManager", () => {
  beforeEach(() => track.mockClear());

  it("shows a skeleton while loading", () => {
    const { container } = setup({ isLoading: true });
    expect(container.querySelector(".MuiSkeleton-root")).toBeInTheDocument();
    expect(screen.queryByText("Prod key")).not.toBeInTheDocument();
  });

  it("lists keys with masked value, status and expiry", () => {
    setup();
    expect(screen.getByText("Prod key")).toBeInTheDocument();
    // Falls back to the key name when no display name is set.
    expect(screen.getByText("old-key")).toBeInTheDocument();
    expect(screen.getByText("sk-****abcd")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("Revoked")).toBeInTheDocument();
    expect(
      screen.getByText(format(new Date("2030-01-02T03:04:05Z"), "dd/MM/yyyy HH:mm:ss")),
    ).toBeInTheDocument();
    expect(screen.getByText("Never expires")).toBeInTheDocument();
  });

  it("shows the empty state with a create action, or a passive one when read-only", () => {
    const { unmount } = setup({ keys: [] });
    expect(screen.getByText("Create your first API key")).toBeInTheDocument();
    expect(screen.getByText("Create a key for this agent.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Create API key/ })).toBeInTheDocument();
    unmount();
    setup({ keys: undefined, readOnly: true });
    expect(screen.getByText("No API keys")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Create API key/ })).not.toBeInTheDocument();
  });

  it("shows a load error with a reload action", () => {
    setup({ isError: true });
    expect(screen.getByText("Couldn't load API keys")).toBeInTheDocument();
    expect(screen.queryByText("Prod key")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload page" })).toBeInTheDocument();
  });

  it("revokes a key only after confirmation", async () => {
    const { onRevoke } = setup();
    fireEvent.click(screen.getAllByRole("button", { name: "Revoke API key" })[0]);
    expect(screen.getByText(/Are you sure you want to revoke "Prod key"\?/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Revoke" }));
    expect(onRevoke).toHaveBeenCalledWith("prod-key");
  });

  it("records a cancelled revoke and does not revoke", () => {
    const { onRevoke } = setup();
    fireEvent.click(screen.getAllByRole("button", { name: "Revoke API key" })[1]);
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onRevoke).not.toHaveBeenCalled();
    expect(track).toHaveBeenCalledWith("confirmation-cancelled", {
      entity: "api-key",
      destructive_action: "revoke",
    });
  });

  it("hides revoke in read-only mode and disables it while revoking", () => {
    const { unmount } = setup({ readOnly: true });
    expect(screen.queryByRole("button", { name: "Revoke API key" })).not.toBeInTheDocument();
    unmount();
    setup({ isRevoking: true });
    screen.getAllByRole("button", { name: "Revoke API key" }).forEach((b) => expect(b).toBeDisabled());
  });

  it("creates a key expiring at the end of the chosen day and shows it once", async () => {
    const { onCreate } = setup();
    onCreate.mockResolvedValue("sk-plaintext");
    fireEvent.click(screen.getByRole("button", { name: /Create API key/ }));
    const create = await screen.findByRole("button", { name: "Create" });
    expect(create).toBeDisabled();
    fireEvent.change(nameInput(), { target: { value: "  Prod Key  " } });
    expect(create).toBeEnabled();
    await act(async () => {
      fireEvent.click(create);
    });

    const [{ displayName, expiresAt }] = onCreate.mock.calls[0];
    expect(displayName).toBe("Prod Key");
    const expiry = new Date(expiresAt);
    expect(format(expiry, "yyyy-MM-dd")).toBe(format(addMonths(new Date(), 1), "yyyy-MM-dd"));
    expect(format(expiry, "HH:mm:ss.SSS")).toBe("23:59:59.999");

    expect(await screen.findByText("You will only see this key once. Copy it now.")).toBeInTheDocument();
    expect(screen.getByDisplayValue("sk-plaintext")).toBeInTheDocument();
    expect(track).toHaveBeenCalledWith("secret-revealed", {
      secret_type: "api-key",
      surface: "issue-banner",
    });
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() =>
      expect(screen.queryByDisplayValue("sk-plaintext")).not.toBeInTheDocument(),
      { timeout: 10000 },
    );
  });

  it("rejects names that reduce to fewer than three key characters", async () => {
    setup();
    fireEvent.click(screen.getByRole("button", { name: /Create API key/ }));
    await screen.findByRole("button", { name: "Create" });
    fireEvent.change(nameInput(), { target: { value: "a!" } });
    expect(screen.getByText("Use at least 3 letters or digits")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
    // Only punctuation: the service picks a random name, so it's allowed.
    fireEvent.change(nameInput(), { target: { value: "!!!" } });
    expect(screen.queryByText("Use at least 3 letters or digits")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeEnabled();
  });

  it("keeps the drawer open when create fails", async () => {
    const { onCreate } = setup();
    onCreate.mockRejectedValue(new Error("boom"));
    fireEvent.click(screen.getByRole("button", { name: /Create API key/ }));
    fireEvent.change(nameInput(), { target: { value: "good name" } });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Create" }));
    });
    expect(nameInput().value).toBe("good name");
    expect(screen.queryByText(/You will only see this key once/)).not.toBeInTheDocument();
  });

  it("closes and resets the drawer on cancel, and shows progress while creating", async () => {
    const { rerender, onCreate } = setup();
    fireEvent.click(screen.getByRole("button", { name: /Create API key/ }));
    fireEvent.change(nameInput(), { target: { value: "draft" } });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    fireEvent.click(screen.getByRole("button", { name: /Create API key/ }));
    expect(nameInput().value).toBe("");
    rerender(
      <ConfirmationDialogProvider>
        <APIKeysManager
          keys={keys}
          isLoading={false}
          isCreating
          emptyDescription=""
          onCreate={onCreate}
          onRevoke={vi.fn()}
        />
      </ConfirmationDialogProvider>,
    );
    expect(screen.getByRole("button", { name: "Creating..." })).toBeDisabled();
  });
});

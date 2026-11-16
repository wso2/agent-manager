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

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type { APIKeyInfo } from "@agent-management-platform/types";
// The component only tracks analytics through api-client, whose auth dependency crashes at
// import time outside a configured app shell.
vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));

import { APIKeysManager, type APIKeysManagerProps } from "./APIKeysManager";
import { ConfirmationDialogProvider } from "../ConfirmationDialog/ConfirmationDialogProvider";

const key = (name: string, expiresAt?: string): APIKeyInfo =>
  ({ name, displayName: name, maskedApiKey: "abc****", status: "active", expiresAt }) as APIKeyInfo;

const renderManager = (props: Partial<APIKeysManagerProps>) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <ConfirmationDialogProvider>
        <APIKeysManager
          keys={[key("ci-key")]}
          isLoading={false}
          isCreating={false}
          emptyDescription="Create a key."
          onCreate={vi.fn()}
          onRevoke={vi.fn()}
          {...props}
        />
      </ConfirmationDialogProvider>
    </ThemeProvider>,
  );

describe("APIKeysManager regenerate", () => {
  it("is not offered without onRotate", () => {
    renderManager({});
    expect(screen.queryByLabelText("Regenerate API key")).toBeNull();
  });

  it("regenerates a key once confirmed and shows the new value once", async () => {
    const onRotate = vi.fn().mockResolvedValue("new-secret-value");
    renderManager({ onRotate });

    fireEvent.click(screen.getByLabelText("Regenerate API key"));
    fireEvent.click(await screen.findByRole("button", { name: "Regenerate" }));

    await waitFor(() => expect(onRotate).toHaveBeenCalledWith("ci-key"));
    expect(await screen.findByDisplayValue("new-secret-value")).toBeTruthy();
  });

  it("is disabled for an expired key", () => {
    renderManager({ keys: [key("old-key", "2020-01-01T00:00:00Z")], onRotate: vi.fn() });
    expect(screen.getByLabelText("Regenerate API key")).toHaveProperty("disabled", true);
  });

  it("is hidden for a reader", () => {
    renderManager({ onRotate: vi.fn(), readOnly: true });
    expect(screen.queryByLabelText("Regenerate API key")).toBeNull();
  });
});

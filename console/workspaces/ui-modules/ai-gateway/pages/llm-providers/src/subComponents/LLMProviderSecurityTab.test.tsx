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

import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));

import type { LLMProviderResponse } from "@agent-management-platform/types";
import { makeProvider, renderWithProviders } from "../__test__/renderWithProviders";
import { LLMProviderSecurityTab } from "./LLMProviderSecurityTab";

const onUpdate = vi.fn();

const renderTab = (
  providerData: LLMProviderResponse | null | undefined,
  { isLoading = false, access = {} as Record<string, boolean> } = {},
) =>
  renderWithProviders(
    <LLMProviderSecurityTab
      providerData={providerData}
      isLoading={isLoading}
      onUpdate={onUpdate}
      isUpdating={false}
    />,
    { access },
  );

const withApiKey = (key = "X-API-Key", extra: Record<string, unknown> = {}) =>
  makeProvider({
    security: { enabled: true, apiKey: { enabled: true, key, in: "header", ...extra } },
  });

const pickMethod = (label: string) => {
  fireEvent.mouseDown(screen.getByRole("combobox"));
  fireEvent.click(within(screen.getByRole("listbox")).getByRole("option", { name: label }));
};

beforeEach(() => {
  onUpdate.mockReset();
  onUpdate.mockResolvedValue(makeProvider());
});

describe("LLMProviderSecurityTab", () => {
  it("shows the stored API key header and keeps Save disabled while clean", () => {
    renderTab(withApiKey());
    expect(screen.getByRole("combobox")).toHaveTextContent("apiKey");
    expect(screen.getByLabelText("Header Key")).toHaveValue("X-API-Key");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Discard" })).toBeDisabled();
  });

  it("reads a provider without an API key as method None and hides the header field", () => {
    renderTab(makeProvider({ security: { enabled: false, apiKey: { enabled: true, key: "k" } } }));
    expect(screen.getByRole("combobox")).toHaveTextContent("None");
    expect(screen.queryByLabelText("Header Key")).not.toBeInTheDocument();
  });

  it("saves a changed header key with the fixed header location", async () => {
    renderTab(withApiKey());
    fireEvent.change(screen.getByLabelText("Header Key"), { target: { value: " X-Token " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("Updated security settings.");
    expect(onUpdate).toHaveBeenCalledWith({
      security: { enabled: true, apiKey: { enabled: true, key: "X-Token", in: "header" } },
    });
  });

  it("requires a header key when API key authentication is chosen", async () => {
    renderTab(makeProvider());
    expect(screen.getByRole("combobox")).toHaveTextContent("None");
    pickMethod("apiKey");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(
      await screen.findByText("API Key is required when using API Key authentication"),
    ).toBeInTheDocument();
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("switching to None disables the API key", async () => {
    renderTab(withApiKey());
    pickMethod("None");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    expect(onUpdate).toHaveBeenCalledWith({
      security: { enabled: true, apiKey: { enabled: false, key: "", in: "header" } },
    });
  });

  it("reports a failed save", async () => {
    onUpdate.mockRejectedValue(new Error("boom"));
    renderTab(withApiKey());
    fireEvent.change(screen.getByLabelText("Header Key"), { target: { value: "Other" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Failed to update security.")).toBeInTheDocument();
  });

  it("discard restores the saved values", () => {
    renderTab(withApiKey());
    fireEvent.change(screen.getByLabelText("Header Key"), { target: { value: "Other" } });
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(screen.getByLabelText("Header Key")).toHaveValue("X-API-Key");
  });

  it("treats a stored non-header location as an unsaved change", () => {
    renderTab(withApiKey("X-API-Key", { in: "query" }));
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("is read-only without write access", () => {
    renderTab(withApiKey(), { access: { llm_provider_write: false } });
    expect(screen.getByLabelText("Header Key")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("shows skeletons while loading and nothing without a provider", () => {
    const { container, rerenderWith } = renderTab(undefined, { isLoading: true });
    expect(screen.getByText("Authentication")).toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    rerenderWith(
      <LLMProviderSecurityTab providerData={undefined} onUpdate={onUpdate} isUpdating={false} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});

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
import { LLMProviderConnectionTab } from "./LLMProviderConnectionTab";

const onUpdate = vi.fn();

const renderTab = (
  providerData: LLMProviderResponse | null | undefined = makeProvider(),
  { valuePrefix = "", isLoading = false, access = {} as Record<string, boolean> } = {},
) =>
  renderWithProviders(
    <LLMProviderConnectionTab
      providerData={providerData}
      valuePrefix={valuePrefix}
      isLoading={isLoading}
      onUpdate={onUpdate}
      isUpdating={false}
    />,
    { access },
  );

const endpoint = () => screen.getByLabelText("Provider Endpoint");
const save = () => screen.getByRole("button", { name: "Save" });

beforeEach(() => {
  onUpdate.mockReset();
  onUpdate.mockResolvedValue(makeProvider());
});

describe("LLMProviderConnectionTab", () => {
  it("shows the saved connection with a masked credential", () => {
    renderTab(makeProvider({ resilience: { timeout: "30s", idleTimeout: "5m" } }));
    expect(endpoint()).toHaveValue("https://api.openai.com/v1");
    expect(screen.getByLabelText("Authentication Header")).toHaveValue("Authorization");
    expect(screen.getByLabelText("Credentials")).toHaveValue("••••••••••••");
    expect(screen.getByLabelText("Request Timeout")).toHaveValue("30s");
    expect(screen.getByLabelText("Idle Timeout")).toHaveValue("5m");
    expect(screen.getByRole("combobox")).toHaveTextContent("API Key");
    expect(save()).toBeDisabled();
  });

  it("saves a new endpoint, keeps the stored credential, and re-syncs deployed gateways", async () => {
    renderTab(makeProvider({ gateways: ["gw-1"] }));
    fireEvent.change(endpoint(), { target: { value: "https://llm.example.com " } });
    fireEvent.click(save());

    await screen.findByText("Connection updated successfully.");
    expect(onUpdate).toHaveBeenCalledWith({
      upstream: {
        main: {
          url: "https://llm.example.com",
          auth: { type: "api-key", header: "Authorization", value: "Bearer secret" },
        },
      },
      resilience: undefined,
      gateways: ["gw-1"],
    });
  });

  it("prefixes a newly typed credential with the template's value prefix", async () => {
    renderTab(makeProvider(), { valuePrefix: "Bearer " });
    const cred = screen.getByLabelText("Credentials");
    fireEvent.focus(cred);
    expect(cred).toHaveValue("");
    fireEvent.change(cred, { target: { value: "sk-new" } });
    fireEvent.change(screen.getByLabelText("Request Timeout"), { target: { value: "10s" } });
    fireEvent.click(save());

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    expect(onUpdate.mock.calls[0][0]).toMatchObject({
      upstream: { main: { auth: { value: "Bearer sk-new" } } },
      resilience: { timeout: "10s", idleTimeout: undefined },
      gateways: undefined,
    });
    // The credential is masked again after a successful save.
    expect(await screen.findByDisplayValue("••••••••••••")).toBeInTheDocument();
  });

  it("sends an empty auth block when authentication is set to None", async () => {
    renderTab();
    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(within(screen.getByRole("listbox")).getByRole("option", { name: "None" }));
    fireEvent.click(save());

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    expect(onUpdate.mock.calls[0][0].upstream.main.auth).toEqual({
      type: "none",
      header: "",
      value: "",
    });
  });

  it("validates the endpoint on blur and blocks saving", () => {
    renderTab();
    fireEvent.change(endpoint(), { target: { value: "not a url" } });
    fireEvent.blur(endpoint());
    expect(screen.getByText("Please enter a valid URL")).toBeInTheDocument();
    expect(save()).toBeDisabled();

    fireEvent.change(endpoint(), { target: { value: "" } });
    expect(screen.getByText("Provider Endpoint is required")).toBeInTheDocument();

    fireEvent.change(endpoint(), { target: { value: "https://ok.example.com" } });
    expect(screen.queryByText("Provider Endpoint is required")).not.toBeInTheDocument();
    expect(save()).toBeEnabled();
  });

  it("rejects timeouts that are not durations", () => {
    renderTab();
    const timeout = screen.getByLabelText("Request Timeout");
    fireEvent.change(timeout, { target: { value: "ten" } });
    fireEvent.blur(timeout);
    expect(screen.getByText("Enter a duration like 5s, 500ms, or 1m")).toBeInTheDocument();
    expect(save()).toBeDisabled();

    const idle = screen.getByLabelText("Idle Timeout");
    fireEvent.change(idle, { target: { value: "x" } });
    fireEvent.blur(idle);
    expect(screen.getAllByText("Enter a duration like 5s, 500ms, or 1m")).toHaveLength(2);

    fireEvent.change(timeout, { target: { value: "5s" } });
    fireEvent.change(idle, { target: { value: "1m" } });
    expect(screen.queryByText("Enter a duration like 5s, 500ms, or 1m")).not.toBeInTheDocument();
  });

  it("reports a failed save", async () => {
    onUpdate.mockRejectedValue(new Error("nope"));
    renderTab();
    fireEvent.change(screen.getByLabelText("Authentication Header"), { target: { value: "X-Key" } });
    fireEvent.click(save());
    expect(await screen.findByText("Failed to update connection.")).toBeInTheDocument();
  });

  it("discard restores the saved connection", () => {
    renderTab();
    fireEvent.change(endpoint(), { target: { value: "https://changed.example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(endpoint()).toHaveValue("https://api.openai.com/v1");
    expect(save()).toBeDisabled();
  });

  it("hides Save and Discard without write access", () => {
    renderTab(makeProvider(), { access: { llm_provider_write: false } });
    expect(endpoint()).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("renders nothing without a provider and a placeholder while loading", () => {
    const { container, rerenderWith } = renderTab(null);
    expect(container).toBeEmptyDOMElement();
    rerenderWith(
      <LLMProviderConnectionTab
        providerData={undefined}
        isLoading
        onUpdate={onUpdate}
        isUpdating={false}
      />,
    );
    expect(screen.queryByLabelText("Provider Endpoint")).not.toBeInTheDocument();
    expect(container).not.toBeEmptyDOMElement();
  });
});

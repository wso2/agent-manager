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

import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
}));

import type {
  LLMProviderResponse,
  LLMRateLimitingConfig,
} from "@agent-management-platform/types";
import {
  makeProvider,
  makeSpec,
  renderWithProviders,
} from "../__test__/renderWithProviders";
import { LLMProviderRateLimitingTab } from "./LLMProviderRateLimitingTab";

// Oxygen UI pages are slow to render under a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });

const onUpdate = vi.fn();

const renderTab = (
  providerData: LLMProviderResponse | null | undefined,
  { path = "/org/acme/llm-providers/view/p-1", access = {} as Record<string, boolean>, isLoading = false } = {},
) =>
  renderWithProviders(
    <LLMProviderRateLimitingTab
      providerData={providerData}
      isLoading={isLoading}
      onUpdate={onUpdate}
      isUpdating={false}
    />,
    { path, access },
  );

const withLimits = (rateLimiting: LLMRateLimitingConfig, openapi?: string) =>
  makeProvider({ rateLimiting, openapi });

// Switches render in card order: Request Counts, Token Count, Cost.
const switches = () => {
  const asSwitch = screen.queryAllByRole("switch", { hidden: true });
  return asSwitch.length ? asSwitch : screen.getAllByRole("checkbox", { hidden: true });
};
const save = () => screen.getByRole("button", { name: "Save" });

beforeEach(() => {
  onUpdate.mockReset();
  onUpdate.mockResolvedValue(makeProvider());
});

describe("LLMProviderRateLimitingTab", () => {
  it("loads a provider-wide limit into the matching card", () => {
    renderTab(
      withLimits({
        providerLevel: {
          global: { request: { enabled: true, count: 100, reset: { duration: 2, unit: "minute" } } },
        },
      }),
    );
    expect(screen.getByText("Request Counts")).toBeInTheDocument();
    expect(screen.getByText("Enabled")).toBeInTheDocument();
    expect(screen.getByLabelText("Quota (requests)")).toHaveValue(100);
    expect(switches()[0]).toBeChecked();
    expect(switches()[1]).not.toBeChecked();
    expect(save()).toBeDisabled();
  });

  it("saves a newly enabled provider-wide request limit", async () => {
    renderTab(makeProvider());
    fireEvent.click(switches()[0]);
    fireEvent.change(screen.getByLabelText("Quota (requests)"), { target: { value: "500.7" } });
    fireEvent.click(save());

    await screen.findByText("Rate limits updated successfully.");
    expect(onUpdate).toHaveBeenCalledWith({
      rateLimiting: {
        providerLevel: {
          global: { request: { enabled: true, count: 500, reset: { duration: 1, unit: "hour" } } },
          resourceWise: undefined,
        },
        consumerLevel: { global: {}, resourceWise: undefined },
      },
    });
  });

  it("saves a cost budget with decimals", async () => {
    renderTab(makeProvider());
    fireEvent.click(switches()[2]);
    fireEvent.change(screen.getByLabelText("Budget (USD)"), { target: { value: "12.5" } });
    fireEvent.click(save());

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    expect(onUpdate.mock.calls[0][0].rateLimiting.providerLevel.global).toEqual({
      cost: { enabled: true, amount: 12.5, reset: { duration: 1, unit: "day" } },
    });
  });

  it("validates quotas and durations of enabled criteria", async () => {
    renderTab(makeProvider());
    fireEvent.click(switches()[0]);
    fireEvent.change(screen.getAllByLabelText("Duration")[0], { target: { value: "0" } });
    fireEvent.click(save());

    expect(await screen.findByText("Quota must be >= 1")).toBeInTheDocument();
    expect(screen.getByText("Duration must be a whole number >= 1")).toBeInTheDocument();
    expect(onUpdate).not.toHaveBeenCalled();
  });

  it("saves a per-consumer limit from the Consumer Level tab", async () => {
    renderTab(makeProvider());
    fireEvent.click(screen.getByRole("tab", { name: "Consumer Level" }));
    expect(screen.getByText(/Limits applied per agent/)).toBeInTheDocument();
    fireEvent.click(switches()[1]);
    fireEvent.change(screen.getByLabelText("Quota (tokens)"), { target: { value: "9000" } });
    fireEvent.click(save());

    await waitFor(() => expect(onUpdate).toHaveBeenCalled());
    expect(onUpdate.mock.calls[0][0].rateLimiting.consumerLevel).toEqual({
      global: { token: { enabled: true, count: 9000, reset: { duration: 1, unit: "hour" } } },
      resourceWise: undefined,
    });
  });

  it("jumps to the Consumer Level tab when its limits are invalid", async () => {
    renderTab(makeProvider());
    fireEvent.click(screen.getByRole("tab", { name: "Consumer Level" }));
    fireEvent.click(switches()[0]);
    fireEvent.click(screen.getByRole("tab", { name: "Provider Level" }));
    fireEvent.click(save());

    expect(await screen.findByText("Quota must be >= 1")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Consumer Level" })).toHaveAttribute("aria-selected", "true");
  });

  it("reports a failed save and discards back to the saved limits", async () => {
    onUpdate.mockRejectedValue(new Error("x"));
    renderTab(makeProvider());
    fireEvent.click(switches()[0]);
    fireEvent.change(screen.getByLabelText("Quota (requests)"), { target: { value: "5" } });
    fireEvent.click(save());
    expect(await screen.findByText("Failed to update rate limits.")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(switches()[0]).not.toBeChecked();
    expect(save()).toBeDisabled();
  });

  it("refuses to save provider-wide and per-resource values together", async () => {
    // ?mode=global overrides the stored per-resource configuration.
    renderTab(
      withLimits({
        providerLevel: {
          resourceWise: {
            default: { request: { enabled: true, count: 10, reset: { duration: 1, unit: "hour" } } },
            resources: [],
          },
        },
      }),
      { path: "/org/acme/llm-providers/view/p-1?mode=global" },
    );
    fireEvent.click(screen.getByRole("tab", { name: "Consumer Level" }));
    fireEvent.click(switches()[0]);
    fireEvent.change(screen.getByLabelText("Quota (requests)"), { target: { value: "3" } });
    fireEvent.click(save());

    expect(
      await screen.findByText(/cannot have both Provider-wide and Per Resource values/),
    ).toBeInTheDocument();
    expect(onUpdate).not.toHaveBeenCalled();
  });

  describe("per resource", () => {
    const spec = makeSpec(["POST /chat/completions", "GET /models", "POST /embeddings"]);
    const perResource = () =>
      withLimits(
        {
          providerLevel: {
            resourceWise: {
              default: { token: { enabled: true, count: 1000, reset: { duration: 1, unit: "day" } } },
              resources: [
                {
                  resource: "POST-/chat/completions",
                  limit: { request: { enabled: true, count: 20, reset: { duration: 1, unit: "minute" } } },
                },
              ],
            },
          },
        },
        spec,
      );

    it("lists the spec's resources and marks the overridden one", () => {
      renderTab(perResource());
      expect(screen.getByText("Default Resource Limit")).toBeInTheDocument();
      expect(screen.getByText("/chat/completions")).toBeInTheDocument();
      expect(screen.getByText("/models")).toBeInTheDocument();
      expect(screen.getAllByText("Custom")).toHaveLength(1);
      expect(screen.getByLabelText("Quota (tokens)")).toHaveValue(1000);
      // Per resource values exist, so switching to provider-wide is blocked.
      expect(screen.getByRole("button", { name: "Provider-wide" })).toBeDisabled();
    });

    it("filters resources by search", () => {
      renderTab(perResource());
      fireEvent.change(screen.getByPlaceholderText("Search resources"), { target: { value: "embed" } });
      expect(screen.getByText("/embeddings")).toBeInTheDocument();
      expect(screen.queryByText("/models")).not.toBeInTheDocument();

      fireEvent.change(screen.getByPlaceholderText("Search resources"), { target: { value: "zzz" } });
      expect(screen.getByText("No resources match your search")).toBeInTheDocument();
    });

    it("saves an override added to another resource", async () => {
      renderTab(perResource());
      fireEvent.click(screen.getByText("/models"));
      // Default block (3) then the expanded /models block (3).
      fireEvent.click(switches()[3]);
      const quotas = screen.getAllByRole("spinbutton", { hidden: true });
      // Default block: 3 quota + 3 duration inputs; /models' request quota follows.
      fireEvent.change(quotas[6], { target: { value: "7" } });
      fireEvent.click(save());

      await waitFor(() => expect(onUpdate).toHaveBeenCalled());
      const providerLevel = onUpdate.mock.calls[0][0].rateLimiting.providerLevel;
      expect(providerLevel.global).toBeUndefined();
      expect(providerLevel.resourceWise.default).toEqual({
        token: { enabled: true, count: 1000, reset: { duration: 1, unit: "day" } },
      });
      expect(providerLevel.resourceWise.resources).toEqual(
        expect.arrayContaining([
          {
            resource: "GET-/models",
            limit: { request: { enabled: true, count: 7, reset: { duration: 1, unit: "hour" } } },
          },
          {
            resource: "POST-/chat/completions",
            limit: { request: { enabled: true, count: 20, reset: { duration: 1, unit: "minute" } } },
          },
        ]),
      );
    });

    it("says when the spec defines no resources", () => {
      renderTab(
        withLimits({ providerLevel: { resourceWise: { default: {}, resources: [] } } }),
      );
      expect(screen.getByText("No resources found")).toBeInTheDocument();
      expect(screen.queryByPlaceholderText("Search resources")).not.toBeInTheDocument();
    });
  });

  it("is read-only without write access", () => {
    renderTab(makeProvider(), { access: { llm_provider_write: false } });
    expect(switches()[0]).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("renders nothing without a provider and placeholders while loading", () => {
    const { container, rerenderWith } = renderTab(undefined);
    expect(container).toBeEmptyDOMElement();
    rerenderWith(
      <LLMProviderRateLimitingTab
        providerData={makeProvider()}
        isLoading
        onUpdate={onUpdate}
        isUpdating={false}
      />,
    );
    expect(screen.queryByRole("tab", { name: "Provider Level" })).not.toBeInTheDocument();
    expect(container).not.toBeEmptyDOMElement();
  });
});

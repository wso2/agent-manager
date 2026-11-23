/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProviderSelectDrawer, type ProviderOption } from "./ProviderSelectDrawer";

const providers: ProviderOption[] = [
  {
    uuid: "u1",
    id: "openai-prov",
    name: "OpenAI Prov",
    template: "openai",
    deployments: [
      { gatewayName: "gw-old", deployedAt: "2020-01-01T00:00:00Z" },
      { gatewayName: "gw-new", deployedAt: new Date().toISOString() },
    ],
    rateLimiting: {
      consumerLevel: {
        globalEnabled: true,
        request: { limit: 1000, resetDuration: 1, resetUnit: "minute" },
        token: { limit: 50000, resetDuration: 2, resetUnit: "hour" },
        cost: { limit: 0.005, resetDuration: 1, resetUnit: "day" },
      },
    },
    policies: ["pii", "toxicity", "jailbreak", "prompt-guard"],
  },
  {
    uuid: "u2",
    id: "claude-prov",
    name: "Claude Prov",
    template: "anthropic",
    rateLimiting: {
      consumerLevel: { globalEnabled: false },
      providerLevel: { globalEnabled: true, cost: { limit: 25, resetUnit: "month" } },
    },
  },
  {
    uuid: "u3",
    id: "local-prov",
    name: "Local Prov",
    rateLimiting: {
      consumerLevel: { globalEnabled: false },
      providerLevel: { globalEnabled: false },
    },
  },
  { uuid: "u4", id: "plain", name: "Plain Prov", rateLimiting: { providerLevel: { globalEnabled: true } } },
];

const templateMap = new Map([
  ["openai", { displayName: "OpenAI", logoUrl: "/openai.svg" }],
]);

const renderDrawer = (props: Partial<Parameters<typeof ProviderSelectDrawer>[0]> = {}) => {
  const onSelect = vi.fn();
  const onClose = vi.fn();
  render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }} initialEntries={["/org/acme/x"]}>
      <Routes>
        <Route
          path="/org/:orgId/x"
          element={
            <ProviderSelectDrawer
              open
              onClose={onClose}
              providers={providers}
              templateMap={templateMap}
              selectedUuid="u2"
              subtitle="Pick one for dev."
              onSelect={onSelect}
              {...props}
            />
          }
        />
      </Routes>
    </MemoryRouter>,
  );
  return { onSelect, onClose };
};

afterEach(() => {
  vi.useRealTimers();
});

describe("ProviderSelectDrawer", () => {
  it("summarizes each provider's quota, guardrails and latest deployment", () => {
    renderDrawer();
    expect(screen.getByText("Pick one for dev.")).toBeInTheDocument();
    // Per-consumer limits.
    expect(screen.getByText("Your Quota:")).toBeInTheDocument();
    expect(screen.getByText("Requests: 1,000/min · Tokens: 50,000/2 hr · Budget: $0.005000/day")).toBeInTheDocument();
    expect(screen.getByText("pii, toxicity, jailbreak +1 more")).toBeInTheDocument();
    expect(screen.getByText(/Deployed/)).toBeInTheDocument();
    // Falls back to the org-wide provider limit.
    expect(screen.getAllByText("Your Quota (org-wide limit):")).toHaveLength(2);
    expect(screen.getByText("Budget: $25.00/month")).toBeInTheDocument();
    expect(screen.getByText("Enabled (no numeric limits set)")).toBeInTheDocument();
    expect(screen.getByText("Configured (disabled)")).toBeInTheDocument();
    expect(screen.getAllByText("None").length).toBeGreaterThan(0);
  });

  it("marks the selected provider", () => {
    renderDrawer();
    expect(screen.getByRole("button", { name: "Claude Prov. Selected" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "OpenAI Prov. Click to select" })).toBeInTheDocument();
  });

  it("selects a provider and closes", () => {
    const { onSelect, onClose } = renderDrawer();
    fireEvent.click(screen.getByRole("button", { name: "Local Prov. Click to select" }));
    expect(onSelect).toHaveBeenCalledWith("u3");
    expect(onClose).toHaveBeenCalled();
  });

  it("filters by name or template after a short pause", () => {
    vi.useFakeTimers();
    renderDrawer();
    fireEvent.change(screen.getByPlaceholderText("Search providers"), { target: { value: "openai" } });
    // Not applied until the debounce fires.
    expect(screen.getByText("Claude Prov")).toBeInTheDocument();
    act(() => {
      vi.advanceTimersByTime(300);
    });
    expect(screen.getByText("OpenAI Prov")).toBeInTheDocument();
    expect(screen.queryByText("Claude Prov")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search providers"), { target: { value: "zzz" } });
    act(() => {
      vi.advanceTimersByTime(300);
    });
    expect(screen.getByText("No service providers match your search")).toBeInTheDocument();
  });

  it("says when the catalog is empty and links to adding a provider", () => {
    renderDrawer({ providers: [] });
    expect(screen.getByText("No service providers available")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Add LLM provider/ })).toHaveAttribute(
      "href",
      expect.stringContaining("/org/acme/llm-providers/add"),
    );
  });
});

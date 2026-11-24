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

import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({
  consumers: { data: undefined as unknown, isLoading: false, error: null as unknown },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useListLLMProviderConsumers: () => hoisted.consumers,
  useOrgAgentDisplayNames: () => ({
    resolveAgentName: (_project: string, agent: string) => `${agent} (display)`,
    resolveProjectName: (project: string) => `${project} project`,
  }),
}));

import { renderWithProviders } from "../__test__/renderWithProviders";
import { LLMProviderConsumersTab } from "./LLMProviderConsumersTab";

const renderTab = (orgName: string | null = "acme") =>
  renderWithProviders(
    <LLMProviderConsumersTab orgName={orgName ?? undefined} providerId="p-1" />,
  );

beforeEach(() => {
  hoisted.consumers = { data: undefined, isLoading: false, error: null };
});

describe("LLMProviderConsumersTab", () => {
  it("lists consuming agents with their display and project names", () => {
    hoisted.consumers.data = {
      consumers: [
        { consumerName: "support-bot", consumerType: "agent", projectName: "shop" },
        { consumerName: "triage", consumerType: "agent", projectName: "ops" },
      ],
    };
    renderTab();
    expect(screen.getByText("support-bot (display)")).toBeInTheDocument();
    expect(screen.getByText("shop project")).toBeInTheDocument();
    expect(screen.getByText("triage (display)")).toBeInTheDocument();
    expect(screen.getAllByText("Agent")).toHaveLength(2);
  });

  it("opens the consuming agent's configuration", () => {
    hoisted.consumers.data = {
      consumers: [{ consumerName: "support-bot", consumerType: "agent", projectName: "shop" }],
    };
    renderTab();
    // Row actions stay visibility:hidden until the row is hovered, which also
    // hides their accessible name, so find the button by its label.
    fireEvent.click(screen.getByLabelText("Go to agent support-bot (display)"));
    expect(
      screen.getByText("at /org/acme/project/shop/agents/support-bot/configure"),
    ).toBeInTheDocument();
  });

  it("hides the link without an organization", () => {
    hoisted.consumers.data = {
      consumers: [{ consumerName: "support-bot", consumerType: "agent", projectName: "shop" }],
    };
    renderTab(null);
    expect(screen.getByText("support-bot (display)")).toBeInTheDocument();
    expect(screen.queryByLabelText(/Go to/)).not.toBeInTheDocument();
  });

  it("shows an empty state when nothing consumes the provider", () => {
    hoisted.consumers.data = { consumers: [] };
    renderTab();
    expect(screen.getByText("No consumers yet")).toBeInTheDocument();
  });

  it("shows an error when consumers fail to load", () => {
    hoisted.consumers.error = new Error("x");
    renderTab();
    expect(screen.getByText("Failed to load consumers.")).toBeInTheDocument();
  });

  it("shows a spinner while loading", () => {
    hoisted.consumers.isLoading = true;
    renderTab();
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });
});

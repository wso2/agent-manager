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

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const envState = vi.hoisted(() => ({
  value: { environments: [] as unknown[], isLoading: false, isError: false },
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  useOrgEnvironmentsState: () => envState.value,
}));
vi.mock("./A2AAgentCardPanel", () => ({
  A2AAgentCardPanel: ({ envId }: { envId: string }) => <div>{`card panel ${envId}`}</div>,
}));

import { A2AConfigureTab } from "./A2AConfigureTab";

const renderTab = (external = false, search = "") =>
  render(
    <MemoryRouter
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      initialEntries={[`/configure${search}`]}
    >
      <A2AConfigureTab orgId="o" projectId="p" agentId="a" external={external} />
    </MemoryRouter>,
  );

beforeEach(() => {
  envState.value = {
    environments: [{ name: "dev", displayName: "Development" }, { name: "prod" }],
    isLoading: false,
    isError: false,
  };
});

describe("A2AConfigureTab", () => {
  it("shows a spinner while environments load", () => {
    envState.value = { environments: [], isLoading: true, isError: false };
    renderTab();
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
  });

  it("reports a failure to load environments", () => {
    envState.value = { environments: [], isLoading: false, isError: true };
    renderTab();
    expect(screen.getByText("Failed to load environments. Please try again.")).toBeInTheDocument();
  });

  it("says when there are no environments", () => {
    envState.value = { environments: [], isLoading: false, isError: false };
    renderTab();
    expect(screen.getByText("No environments are set up for this project.")).toBeInTheDocument();
  });

  it("explains the gateway-fetched card for a platform agent", () => {
    renderTab();
    expect(screen.getByText(/fetches this agent's card from its gateway/)).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "prod" })).toBeInTheDocument();
    expect(screen.getByText("card panel dev")).toBeInTheDocument();
  });

  it("asks an external agent to register its card URL and falls back from an unknown env", () => {
    renderTab(true, "?env=missing");
    expect(screen.getByText(/register its agent card URL/)).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Development" })).toHaveAttribute("aria-selected", "true");
  });
});

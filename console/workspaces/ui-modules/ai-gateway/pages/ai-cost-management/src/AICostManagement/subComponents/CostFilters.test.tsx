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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { CostByGroup } from "@agent-management-platform/types";

vi.setConfig({ testTimeout: 60_000 });

const hoisted = vi.hoisted(() => ({
  state: {
    data: undefined as unknown,
    isLoading: false,
    error: undefined as unknown,
  },
  useCosts: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCosts: (...args: unknown[]) => {
    hoisted.useCosts(...args);
    return hoisted.state;
  },
}));
vi.mock("../useEntityNames", () => ({
  useEntityNames: () => ({
    project: { payments: "Payments" },
    agent: { "payments/refund-bot": "Refund Bot" },
    llmProvider: {},
  }),
}));

import { CostFilterBar, scopeFilter, useCostSelection } from "./CostFilters";

const group = (id: string | null, project?: string): CostByGroup => ({
  id,
  project,
  cost: "10.00",
  unpricedRequests: { missingPrice: 0, usageUnavailable: 0 },
  monthly: [],
});

function Harness({
  projectName,
  agentName,
}: {
  projectName?: string;
  agentName?: string;
}) {
  const selection = useCostSelection("acme", projectName, agentName);
  const location = useLocation();
  return (
    <>
      <CostFilterBar selection={selection} />
      <div data-testid="search">{location.search}</div>
      <div data-testid="pickFilter">{selection.pickFilter ?? "none"}</div>
    </>
  );
}

const renderBar = (
  url = "/",
  props: { projectName?: string; agentName?: string } = {},
) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={[url]}>
        <Harness {...props} />
      </MemoryRouter>
    </ThemeProvider>,
  );

beforeEach(() => {
  hoisted.useCosts.mockClear();
  hoisted.state.isLoading = false;
  hoisted.state.error = undefined;
  hoisted.state.data = {
    costByGroup: [group("payments"), group("analytics"), group("zeta")],
  };
});

describe("scopeFilter", () => {
  it("is undefined for the organization", () => {
    expect(scopeFilter()).toBeUndefined();
  });
  it("filters a project", () => {
    expect(scopeFilter("payments")).toEqual(["project:payments"]);
  });
  it("filters an agent within its project", () => {
    expect(scopeFilter("payments", "refund-bot")).toEqual([
      "agent:payments/refund-bot",
    ]);
  });
});

describe("CostFilterBar", () => {
  it("defaults to grouping an organization by project", () => {
    renderBar();
    expect(screen.getByText("Group by: Project")).toBeTruthy();
    expect(screen.getByText("All projects")).toBeTruthy();
    expect(hoisted.useCosts).toHaveBeenCalledWith(
      { orgName: "acme" },
      { groupBy: "project", filter: undefined },
    );
  });

  it("defaults a project page to agents and scopes the query", () => {
    renderBar("/", { projectName: "payments" });
    expect(screen.getByText("Group by: Agent")).toBeTruthy();
    expect(screen.getByText(/in payments/)).toBeTruthy();
    expect(hoisted.useCosts).toHaveBeenCalledWith(
      { orgName: "acme" },
      { groupBy: "agent", filter: ["project:payments"] },
    );
  });

  it("defaults an agent page to models", () => {
    renderBar("/", { projectName: "payments", agentName: "refund-bot" });
    expect(screen.getByText("Group by: Model")).toBeTruthy();
    expect(screen.getByText(/for agent refund-bot/)).toBeTruthy();
  });

  it("ignores a groupBy the page does not offer", () => {
    renderBar("/?groupBy=project", { projectName: "payments" });
    expect(screen.getByText("Group by: Agent")).toBeTruthy();
  });

  it("shows the picked values by display name", () => {
    renderBar("/?groupBy=project&pick=payments&pick=analytics");
    expect(screen.getByText("Payments, analytics")).toBeTruthy();
    expect(screen.getByTestId("pickFilter").textContent).toBe(
      "project:payments,analytics",
    );
    expect(screen.getByPlaceholderText("Add project")).toBeTruthy();
  });

  it("hides the picker when grouping by organization", () => {
    renderBar("/?groupBy=organization");
    expect(screen.queryByLabelText("Projects to show")).toBeNull();
    expect(screen.getByText("The organization")).toBeTruthy();
    expect(screen.getByTestId("pickFilter").textContent).toBe("none");
  });

  it("changing Group by sets the URL and clears picks", () => {
    renderBar("/?groupBy=project&pick=payments");
    fireEvent.mouseDown(screen.getByText("Group by: Project"));
    const list = within(screen.getByRole("listbox"));
    expect(list.getAllByRole("option")).toHaveLength(7);
    fireEvent.click(list.getByRole("option", { name: "Model" }));
    expect(screen.getByTestId("search").textContent).toBe("?groupBy=model");
  });

  it("lists options sorted by display name and writes a pick to the URL", () => {
    renderBar("/?groupBy=project");
    fireEvent.mouseDown(screen.getByLabelText("Projects to show"));
    const options = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((o) => o.textContent);
    expect(options).toEqual(["analytics", "Payments", "zeta"]);
    fireEvent.click(screen.getByRole("option", { name: "Payments" }));
    expect(screen.getByTestId("search").textContent).toBe(
      "?groupBy=project&pick=payments",
    );
  });

  it("builds agent options as project/agent", () => {
    hoisted.state.data = {
      costByGroup: [group("refund-bot", "payments"), group(null, "")],
    };
    renderBar("/?groupBy=agent");
    fireEvent.mouseDown(screen.getByLabelText("Agents to show"));
    const options = within(screen.getByRole("listbox"))
      .getAllByRole("option")
      .map((o) => o.textContent);
    expect(options).toEqual(["Payments / Refund Bot", "Unattributed"]);
  });

  it("copes with no cost data", () => {
    hoisted.state.data = undefined;
    renderBar();
    fireEvent.mouseDown(screen.getByLabelText("Projects to show"));
    expect(screen.getByText("No options")).toBeTruthy();
  });
});

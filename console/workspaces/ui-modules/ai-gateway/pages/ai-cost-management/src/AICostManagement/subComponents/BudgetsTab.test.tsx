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

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  Budget,
  BudgetListResponse,
} from "@agent-management-platform/types";

const hoisted = vi.hoisted(() => ({
  state: {
    data: undefined as unknown,
    isLoading: false,
    error: undefined as unknown,
  },
  useListBudgets: vi.fn(),
  access: new Set<string>(),
  confirmDelete: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useListBudgets: (...args: unknown[]) => {
    hoisted.useListBudgets(...args);
    return hoisted.state;
  },
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureManagement: () => ({
    hasAccess: (feature: string) => hoisted.access.has(feature),
  }),
}));
vi.mock("./useConfirmDeleteBudget", () => ({
  useConfirmDeleteBudget: () => ({
    confirmDelete: hoisted.confirmDelete,
    isDeleting: false,
  }),
}));
vi.mock("./ImportBudgetsDialog", () => ({ ImportBudgetsDialog: () => null }));

import { BudgetsTab } from "./BudgetsTab";

const budget = (
  name: string,
  over: Partial<Budget["spec"]> = {},
  status?: Budget["status"],
): Budget => ({
  apiVersion: "gateway.api-platform.wso2.com/v1",
  kind: "Budget",
  metadata: { name, labels: ["finance", "eu"] },
  spec: {
    displayName: `Budget ${name}`,
    status: "active",
    limit: { amount: "700.00", period: "month", action: "block", blockAt: 100 },
    filters: [],
    exemptions: [],
    warnings: { thresholds: [70, 90, 100], channels: ["finance-alerts"] },
    ...over,
  },
  status: status ?? {
    origin: "controlPlane",
    currentPeriod: {
      periodStart: "2026-11-01",
      spent: "329.00",
      unpricedRequests: 0,
    },
    createdAt: "2026-11-01T00:00:00Z",
    updatedAt: "2026-11-01T00:00:00Z",
  },
});

const list = (
  budgets: Budget[],
  total = budgets.length,
): BudgetListResponse => ({
  budgets,
  total,
  limit: 10,
  offset: 0,
});

const renderTab = (
  props: Partial<React.ComponentProps<typeof BudgetsTab>> = {},
) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter>
        <BudgetsTab orgName="acme" onOpen={vi.fn()} {...props} />
      </MemoryRouter>
    </ThemeProvider>,
  );

const lastCall = () =>
  hoisted.useListBudgets.mock.calls[
    hoisted.useListBudgets.mock.calls.length - 1
  ];

beforeEach(() => {
  hoisted.useListBudgets.mockClear();
  hoisted.confirmDelete.mockClear();
  hoisted.access = new Set(["cost_delete"]);
  hoisted.state = {
    data: list([budget("azure-eu-monthly")]),
    isLoading: false,
    error: undefined,
  };
});

describe("BudgetsTab", () => {
  it("shows a budget's name, limit, period, action, spend, warnings and status", () => {
    renderTab();
    const row = screen
      .getByText("Budget azure-eu-monthly")
      .closest("tr") as HTMLElement;
    expect(within(row).getByText("azure-eu-monthly")).toBeInTheDocument();
    expect(within(row).getByText("$700")).toBeInTheDocument();
    expect(within(row).getByText("Monthly")).toBeInTheDocument();
    expect(within(row).getByText("Block")).toBeInTheDocument();
    expect(within(row).getByText("$329")).toBeInTheDocument();
    expect(within(row).getByText("47% of the limit")).toBeInTheDocument();
    for (const t of ["70%", "90%", "100%"])
      expect(within(row).getByText(t)).toBeInTheDocument();
    expect(within(row).getByText("Active")).toBeInTheDocument();
    // Labels are searched for, and shown on the budget's page, not in the list.
    expect(within(row).queryByText("finance")).not.toBeInTheDocument();
  });

  it("marks an inactive budget and an allow budget", () => {
    hoisted.state.data = list([
      budget("quiet", {
        status: "inactive",
        limit: { amount: "3.00", period: "week", action: "allow" },
        warnings: undefined,
      }),
    ]);
    renderTab();
    const row = screen.getByText("Budget quiet").closest("tr") as HTMLElement;
    expect(within(row).getByText("Inactive")).toBeInTheDocument();
    expect(within(row).getByText("Allow")).toBeInTheDocument();
    expect(within(row).getByText("Weekly")).toBeInTheDocument();
    // No warnings: the cell says so.
    expect(
      within(row).getByText("None", { selector: "td" }),
    ).toBeInTheDocument();
  });

  it("shows three warning chips and the rest as +N", () => {
    hoisted.state.data = list([
      budget("many", {
        warnings: { thresholds: [50, 70, 80, 90, 100], channels: ["a"] },
      }),
    ]);
    renderTab();
    const row = screen.getByText("Budget many").closest("tr") as HTMLElement;
    for (const t of ["50%", "70%", "80%"])
      expect(within(row).getByText(t)).toBeInTheDocument();
    expect(within(row).queryByText("90%")).not.toBeInTheDocument();
    expect(within(row).getByText("+2")).toBeInTheDocument();
  });

  it("keeps a long name to one line, with the whole name on hover", () => {
    const long = "A very long budget name ".repeat(8).trim();
    hoisted.state.data = list([budget("long-one", { displayName: long })]);
    renderTab();
    expect(screen.getByText(long)).toHaveAttribute("title", long);
  });

  it("says a budget with no spend yet has none", () => {
    hoisted.state.data = list([
      budget(
        "fresh",
        {},
        {
          origin: "controlPlane",
          createdAt: "2026-11-26T00:00:00Z",
          updatedAt: "2026-11-26T00:00:00Z",
        },
      ),
    ]);
    renderTab();
    expect(screen.getByText("No spend yet")).toBeInTheDocument();
  });

  it("does not turn a limit counted separately into a share", () => {
    hoisted.state.data = list([
      budget("per-agent", {
        limit: {
          amount: "90.00",
          period: "day",
          action: "block",
          blockAt: 100,
          splitBy: { type: "agent" },
        },
      }),
    ]);
    renderTab();
    expect(
      screen.getByText("Each agent has its own limit"),
    ).toBeInTheDocument();
    expect(screen.queryByText(/% of the limit/)).not.toBeInTheDocument();
  });

  it("asks for the first page of 10, with no filter, for the organization", () => {
    renderTab();
    expect(lastCall()).toEqual([
      { orgName: "acme" },
      {
        filter: undefined,
        search: undefined,
        status: undefined,
        limit: 10,
        offset: 0,
      },
    ]);
  });

  it("searches on the server once typing pauses", async () => {
    renderTab();
    fireEvent.change(screen.getByPlaceholderText("Search by name or label"), {
      target: { value: "eu" },
    });
    await waitFor(() =>
      expect(lastCall()?.[1]).toMatchObject({ search: "eu", offset: 0 }),
    );
  });

  it("filters by status, and asks for the budgets at their limit as limitReached", () => {
    renderTab();
    fireEvent.mouseDown(screen.getByLabelText("Filter by status"));
    fireEvent.click(screen.getByRole("option", { name: "Limit reached" }));
    expect(lastCall()?.[1]).toMatchObject({
      status: "limitReached",
      offset: 0,
    });

    fireEvent.mouseDown(screen.getByLabelText("Filter by status"));
    fireEvent.click(screen.getByRole("option", { name: "Inactive" }));
    expect(lastCall()?.[1]).toMatchObject({ status: "inactive" });
  });

  it("opens the budget that is clicked", () => {
    const onOpen = vi.fn();
    renderTab({ onOpen });
    fireEvent.click(screen.getByText("Budget azure-eu-monthly"));
    expect(onOpen).toHaveBeenCalledWith("azure-eu-monthly");
  });

  it("offers Delete with the permission, and asks before deleting", () => {
    renderTab();
    // Hidden until the row is hovered; present in the page.
    fireEvent.click(screen.getByText("Delete"));
    expect(hoisted.confirmDelete).toHaveBeenCalledWith(
      "azure-eu-monthly",
      "Budget azure-eu-monthly",
    );
  });

  it("does not offer Delete without the permission", () => {
    hoisted.access = new Set();
    renderTab();
    expect(screen.queryByText("Delete")).not.toBeInTheDocument();
  });

  it("does not offer Delete on a budget made on a gateway", () => {
    hoisted.state.data = list([
      budget(
        "edge-hourly",
        {},
        {
          origin: "gatewayApi",
          currentPeriod: {
            periodStart: "2026-11-01",
            spent: "1.00",
            unpricedRequests: 0,
          },
          createdAt: "2026-11-01T00:00:00Z",
          updatedAt: "2026-11-01T00:00:00Z",
        },
      ),
    ]);
    renderTab();
    expect(screen.queryByText("Delete")).not.toBeInTheDocument();
  });

  it("on a project's page asks for that project's budgets, read-only", () => {
    renderTab({ projectName: "online-shop" });
    expect(lastCall()?.[1]).toMatchObject({ filter: "project:online-shop" });
    expect(screen.queryByText("Delete")).not.toBeInTheDocument();
  });

  it("on an agent's page asks for that agent's budgets", () => {
    renderTab({ projectName: "online-shop", agentName: "returns-bot" });
    expect(lastCall()?.[1]).toMatchObject({
      filter: "agent:online-shop/returns-bot",
    });
  });

  it("explains an empty list, and a search with no match", () => {
    hoisted.state.data = list([]);
    const { unmount } = renderTab();
    expect(screen.getByText("No budgets yet")).toBeInTheDocument();
    unmount();

    hoisted.state.data = list([]);
    renderTab({ projectName: "online-shop" });
    expect(
      screen.getByText("No budgets apply to this project"),
    ).toBeInTheDocument();
  });

  it("tells the user when the list could not be loaded", () => {
    hoisted.state = {
      data: undefined,
      isLoading: false,
      error: new Error("boom"),
    };
    renderTab();
    expect(screen.getByText(/Couldn't load budgets/)).toBeInTheDocument();
  });
});

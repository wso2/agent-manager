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
import { MemoryRouter, useLocation } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Budget } from "@agent-management-platform/types";

const hoisted = vi.hoisted(() => ({
  useBudgetUsage: vi.fn(),
  get: {
    data: undefined as unknown,
    isLoading: false,
    error: undefined as unknown,
    refetch: vi.fn(),
  },
  usage: {
    data: undefined as unknown,
    isLoading: false,
    error: undefined as unknown,
    refetch: vi.fn(),
  },
  update: vi.fn(),
  confirmDelete: vi.fn(),
  access: new Set<string>(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useGetBudget: () => hoisted.get,
  useBudgetUsage: (...args: unknown[]) => {
    hoisted.useBudgetUsage(...args);
    return hoisted.usage;
  },
  useUpdateBudget: () => ({ mutate: hoisted.update, isPending: false }),
  useListLLMProviders: () => ({
    data: {
      providers: [{ id: "openai-production", name: "OpenAI Production" }],
    },
  }),
  useListProjects: () => ({ data: { projects: [] } }),
  useListOrgAgents: () => ({ data: { agents: [] } }),
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

import { BudgetDetail } from "./BudgetDetail";

const budget = (
  over: Partial<Budget["spec"]> = {},
  status?: Partial<NonNullable<Budget["status"]>>,
): Budget => ({
  apiVersion: "gateway.api-platform.wso2.com/v1",
  kind: "Budget",
  metadata: { name: "ui-test-budget", labels: ["uitest"] },
  spec: {
    displayName: "UI test budget",
    description: "Created in a test",
    status: "active",
    limit: { amount: "50.00", period: "month", action: "block", blockAt: 100 },
    filters: [
      {
        type: "attribute",
        expression:
          "(request.Path.startsWith('/lt-mini/')) || (request.Path.startsWith('/lt-claude/'))",
      },
    ],
    exemptions: [{ type: "llmProvider", id: "openai-production" }],
    warnings: { thresholds: [80], channels: ["finance-alerts"] },
    ...over,
  },
  status: {
    origin: "controlPlane",
    currentPeriod: {
      periodStart: "2026-11-01",
      spent: "12.50",
      unpricedRequests: 0,
    },
    createdAt: "2026-11-26T08:00:00Z",
    updatedAt: "2026-11-26T08:00:00Z",
    ...status,
  },
});

/** Shows where the app went, so a click that navigates can be checked. */
function Where() {
  const l = useLocation();
  return <div data-testid="where">{l.pathname}</div>;
}

const renderDetail = (
  props: Partial<React.ComponentProps<typeof BudgetDetail>> = {},
) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter>
        <BudgetDetail
          orgName="acme"
          budgetName="ui-test-budget"
          backHref="/budgets"
          editHref="/budgets/view/ui-test-budget/edit"
          {...props}
        />
        <Where />
      </MemoryRouter>
    </ThemeProvider>,
  );

beforeEach(() => {
  hoisted.useBudgetUsage.mockClear();
  hoisted.update.mockClear();
  hoisted.confirmDelete.mockClear();
  hoisted.get = {
    data: budget(),
    isLoading: false,
    error: undefined,
    refetch: vi.fn(),
  };
  hoisted.usage = {
    data: { periodStart: "2026-11-01", usages: [] },
    isLoading: false,
    error: undefined,
    refetch: vi.fn(),
  };
  hoisted.access = new Set(["cost_read", "cost_write", "cost_delete"]);
});

describe("BudgetDetail", () => {
  it("names the budget, its status and its labels", () => {
    renderDetail();
    expect(screen.getByText("UI test budget")).toBeInTheDocument();
    expect(screen.getByText("Created in a test")).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getAllByText("uitest").length).toBeGreaterThan(0);
  });

  it("shows this period's spend against the limit", () => {
    renderDetail();
    expect(screen.getByText("$12.50")).toBeInTheDocument();
    expect(screen.getByText(/of \$50\.00 used/)).toBeInTheDocument();
  });

  it("says what a Block budget does at the limit", () => {
    renderDetail();
    expect(screen.getByText("Block")).toBeInTheDocument();
    expect(screen.getByText("Blocks at $50.00 (100%).")).toBeInTheDocument();
  });

  it("blocks at the share chosen, not at the whole amount", () => {
    hoisted.get.data = budget({
      limit: { amount: "200.00", period: "week", action: "block", blockAt: 80 },
    });
    renderDetail();
    expect(screen.getByText("Blocks at $160.00 (80%).")).toBeInTheDocument();
  });

  it("says an Allow budget only warns", () => {
    hoisted.get.data = budget({
      limit: { amount: "50.00", period: "month", action: "allow" },
    });
    renderDetail();
    expect(screen.getByText("Only warns.")).toBeInTheDocument();
  });

  it("shows several CEL expressions as alternatives, each on its own", () => {
    renderDetail();
    expect(
      screen.getByText("request.Path.startsWith('/lt-mini/')"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("request.Path.startsWith('/lt-claude/')"),
    ).toBeInTheDocument();
    expect(screen.getByText("or")).toBeInTheDocument();
  });

  it("names a filter with no value, for every type", () => {
    hoisted.get.data = budget({
      filters: [
        { type: "project" },
        { type: "llmProvider" },
        { type: "agent" },
      ],
    });
    renderDetail();
    for (const t of ["All projects", "All LLM providers", "All agents"])
      expect(screen.getByText(t)).toBeInTheDocument();
  });

  it("shows an exempt LLM provider by its name", () => {
    renderDetail();
    expect(
      screen.getByText("LLM provider: OpenAI Production"),
    ).toBeInTheDocument();
  });

  it("shows the warnings and where they go", () => {
    renderDetail();
    expect(screen.getByText("80%")).toBeInTheDocument();
    expect(screen.getByText("finance-alerts")).toBeInTheDocument();
  });

  it("shows a separate-limit budget's breakdown, one line for each value", () => {
    hoisted.get.data = budget({
      limit: {
        amount: "90.00",
        period: "day",
        action: "block",
        blockAt: 100,
        splitBy: { type: "agent" },
      },
      filters: [],
      exemptions: [],
    });
    hoisted.usage.data = {
      periodStart: "2026-11-01",
      usages: [
        {
          usage: { project: "support", agent: "triage" },
          spent: "45.00",
          unpricedRequests: 0,
        },
      ],
    };
    renderDetail();
    expect(screen.getByText("Separate limit per agent")).toBeInTheDocument();
    expect(screen.getByText("Spend breakdown")).toBeInTheDocument();
    expect(screen.getByText("$45.00 of $90.00 · 50%")).toBeInTheDocument();
  });

  it("pages the spend breakdown on the server, 10 at a time", () => {
    hoisted.get.data = budget({
      limit: {
        amount: "90.00",
        period: "day",
        action: "block",
        blockAt: 100,
        splitBy: { type: "agent" },
      },
      filters: [],
      exemptions: [],
    });
    hoisted.usage.data = {
      periodStart: "2026-11-01",
      total: 23,
      limit: 10,
      offset: 0,
      usages: [
        {
          usage: { project: "support", agent: "triage" },
          spent: "45.00",
          unpricedRequests: 0,
        },
      ],
    };
    renderDetail();
    const call = () =>
      hoisted.useBudgetUsage.mock.calls[
        hoisted.useBudgetUsage.mock.calls.length - 1
      ];
    expect(call()?.[1]).toEqual({ limit: 10, offset: 0 });
    expect(screen.getByText("1–10 of 23")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(call()?.[1]).toEqual({ limit: 10, offset: 10 });
  });

  it("warns that an inactive budget counts and blocks nothing, and offers Enable", () => {
    hoisted.get.data = budget({ status: "inactive" });
    renderDetail();
    expect(screen.getByText("Inactive")).toBeInTheDocument();
    expect(
      screen.getByText(
        /Disabled: not counting or blocking until it is enabled/,
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enable" })).toBeInTheDocument();
  });

  it("disables a budget by sending the whole kind with its status flipped", () => {
    renderDetail();
    fireEvent.click(screen.getByRole("button", { name: "Disable" }));
    expect(hoisted.update).toHaveBeenCalledTimes(1);
    const call = hoisted.update.mock.calls[0][0];
    expect(call.params).toEqual({
      orgName: "acme",
      budgetName: "ui-test-budget",
    });
    expect(call.body.kind).toBe("Budget");
    expect(call.body.metadata.name).toBe("ui-test-budget");
    expect(call.body.spec.status).toBe("inactive");
    // Nothing else of the spec is lost on the way.
    expect(call.body.spec.limit.amount).toBe("50.00");
    expect(call.body.spec.exemptions).toEqual([
      { type: "llmProvider", id: "openai-production" },
    ]);
    expect(call.body.spec.warnings.channels).toEqual(["finance-alerts"]);
    // The server's status is not sent back.
    expect(call.body.status).toBeUndefined();
  });

  it("enables an inactive budget", () => {
    hoisted.get.data = budget({ status: "inactive" });
    renderDetail();
    fireEvent.click(screen.getByRole("button", { name: "Enable" }));
    expect(hoisted.update.mock.calls[0][0].body.spec.status).toBe("active");
  });

  it("goes to the edit page", () => {
    renderDetail();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    expect(screen.getByTestId("where")).toHaveTextContent(
      "/budgets/view/ui-test-budget/edit",
    );
  });

  it("asks before deleting, and returns to the list once it is deleted", () => {
    renderDetail();
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(hoisted.confirmDelete).toHaveBeenCalledTimes(1);
    const [name, display, onDeleted] = hoisted.confirmDelete.mock.calls[0];
    expect([name, display]).toEqual(["ui-test-budget", "UI test budget"]);
    act(() => onDeleted());
    expect(screen.getByTestId("where")).toHaveTextContent("/budgets");
  });

  it("offers each action only with its own permission", () => {
    hoisted.access = new Set(["cost_read", "cost_write"]);
    const { unmount } = renderDetail();
    expect(screen.getByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Delete" }),
    ).not.toBeInTheDocument();
    unmount();

    hoisted.access = new Set(["cost_read"]);
    renderDetail();
    expect(
      screen.queryByRole("button", { name: "Edit" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Disable" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Delete" }),
    ).not.toBeInTheDocument();
  });

  it("offers no action on a read-only view", () => {
    renderDetail({ readOnly: true });
    expect(
      screen.queryByRole("button", { name: "Edit" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Delete" }),
    ).not.toBeInTheDocument();
  });

  it("marks a budget made on a gateway, and does not let Disable or Delete change it here", () => {
    hoisted.get.data = budget({}, { origin: "gatewayApi" });
    renderDetail();
    expect(screen.getByText("From a gateway")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Disable" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Edit" })).toBeEnabled();
  });

  it("says when there is no budget of that name", () => {
    hoisted.get = {
      data: undefined,
      isLoading: false,
      error: { status: 404, code: "BUDGET_NOT_FOUND" },
      refetch: vi.fn(),
    };
    renderDetail({ budgetName: "gone" });
    expect(screen.getByText("Budget not found")).toBeInTheDocument();
    expect(
      screen.getByText("There is no budget named gone"),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getAllByRole("button", { name: "Back to budgets" })[0],
    );
    expect(screen.getByTestId("where")).toHaveTextContent("/budgets");
  });

  it("offers Retry when the budget could not be loaded", () => {
    hoisted.get = {
      data: undefined,
      isLoading: false,
      error: new Error("boom"),
      refetch: vi.fn(),
    };
    renderDetail();
    expect(screen.getByText("Couldn't load the budget")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(hoisted.get.refetch).toHaveBeenCalledTimes(1);
  });

  it("links the unpriced requests, only to someone who can read costs", () => {
    hoisted.get.data = budget(
      {},
      {
        currentPeriod: {
          periodStart: "2026-11-01",
          spent: "1.00",
          unpricedRequests: 7,
        },
      },
    );
    const { unmount } = renderDetail({ unpricedHref: "/unpriced" });
    expect(screen.getByText("7")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "View requests" }),
    ).toBeInTheDocument();
    unmount();

    hoisted.access = new Set(["cost_write"]);
    renderDetail({ unpricedHref: "/unpriced" });
    expect(
      screen.queryByRole("link", { name: "View requests" }),
    ).not.toBeInTheDocument();
  });
});

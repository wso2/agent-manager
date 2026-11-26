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
  config: { analyticsDashboardUrl: undefined as string | undefined },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCosts: (...args: unknown[]) => {
    hoisted.useCosts(...args);
    return hoisted.state;
  },
}));
vi.mock("@agent-management-platform/types", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  get globalConfig() {
    return hoisted.config;
  },
}));
vi.mock("../useEntityNames", () => ({
  useEntityNames: () => ({
    project: { payments: "Payments" },
    llmProvider: {},
    agent: {},
  }),
}));
vi.mock("./CostChart", () => ({
  CostChart: ({
    rows,
    series,
  }: {
    rows: { month: string }[];
    series: { name: string }[];
  }) => (
    <div data-testid="chart">
      <span>{rows.map((r) => r.month).join("|")}</span>
      <span>{series.map((s) => s.name).join("|")}</span>
    </div>
  ),
}));

import { CostOverview } from "./CostOverview";

const group = (
  id: string,
  costs: [string, string, string],
  unpriced = { missingPrice: 0, usageUnavailable: 0 },
): CostByGroup =>
  ({
    id,
    cost: String(costs.reduce((a, c) => a + Number(c), 0)),
    unpricedRequests: unpriced,
    monthly: [
      { month: "2026-09", cost: costs[0] },
      { month: "2026-10", cost: costs[1] },
      { month: "2026-11", cost: costs[2] },
    ],
  }) as unknown as CostByGroup;

const Where = () => <span data-testid="where">{useLocation().search}</span>;

const renderOverview = (
  props: Partial<React.ComponentProps<typeof CostOverview>> = {},
  url = "/",
) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={[url]}>
        <CostOverview orgName="acme" unpricedHref="/unpriced" {...props} />
        <Where />
      </MemoryRouter>
    </ThemeProvider>,
  );

beforeEach(() => {
  hoisted.useCosts.mockClear();
  hoisted.config = { analyticsDashboardUrl: undefined };
  hoisted.state = {
    data: {
      costByGroup: [
        group("payments", ["100", "200", "300"], {
          missingPrice: 4,
          usageUnavailable: 2,
        }),
        group("search", ["10", "20", "30"]),
      ],
    },
    isLoading: false,
    error: undefined,
  };
});

describe("CostOverview", () => {
  it("shows the tiles with spend and change against the previous month", () => {
    renderOverview();
    expect(screen.getByText("This month (Nov) so far")).toBeInTheDocument();
    expect(screen.getByText("$330.00")).toBeInTheDocument();
    expect(screen.getByText("+50% vs Oct")).toBeInTheDocument();
    expect(screen.getByText("Last month (Oct)")).toBeInTheDocument();
    expect(screen.getByText("$220.00")).toBeInTheDocument();
    expect(screen.getByText("+100% vs Sep")).toBeInTheDocument();
    expect(screen.getByText("Last 3 months (Sep–Nov)")).toBeInTheDocument();
    expect(screen.getByText("$660.00")).toBeInTheDocument();
    expect(screen.getByText("Includes Nov so far")).toBeInTheDocument();
    expect(
      screen.getByText("Requests charged $0 (Sep–Nov)"),
    ).toBeInTheDocument();
    expect(screen.getByText("6")).toBeInTheDocument();
    expect(screen.getByText(/4 no price · 2 no usage/)).toBeInTheDocument();
  });

  it("reads cost grouped by project for the organization", () => {
    renderOverview();
    expect(hoisted.useCosts).toHaveBeenCalledWith(
      { orgName: "acme" },
      { groupBy: "project", filter: undefined },
    );
  });

  it("charts the groups by display name with the current month so far", () => {
    renderOverview();
    const chart = screen.getByTestId("chart");
    expect(chart).toHaveTextContent("Sep|Oct|Nov (so far)");
    expect(chart).toHaveTextContent("Payments|search");
  });

  it("sums groups beyond the top five as Other", () => {
    hoisted.state.data = {
      costByGroup: ["a", "b", "c", "d", "e", "f", "g"].map((id, i) =>
        group(id, ["1", "1", String(10 - i)]),
      ),
    };
    renderOverview();
    expect(screen.getByTestId("chart")).toHaveTextContent("Other (2)");
  });

  it("names groups that cost nothing under the chart", () => {
    hoisted.state.data = {
      costByGroup: [
        group("payments", ["1", "2", "3"]),
        group("ledger", ["0", "0", "0"]),
      ],
    };
    renderOverview();
    expect(
      screen.getByText("Not shown (charged $0): ledger."),
    ).toBeInTheDocument();
    expect(screen.getByText("UTC months · last 3 months")).toBeInTheDocument();
  });

  it("omits the change captions without a previous month's cost", () => {
    hoisted.state.data = {
      costByGroup: [group("payments", ["0", "0", "50"])],
    };
    renderOverview();
    expect(screen.queryByText(/% vs/)).not.toBeInTheDocument();
  });

  it("links Details to the unpriced page with the selection", () => {
    renderOverview({}, "/?groupBy=project&pick=payments");
    expect(screen.getByRole("link", { name: "Details" })).toHaveAttribute(
      "href",
      "/unpriced?groupBy=project&pick=payments",
    );
  });

  it("has no Details link without an unpriced page", () => {
    renderOverview({ unpricedHref: undefined });
    expect(screen.queryByRole("link", { name: "Details" })).toBeNull();
  });

  it("limits the figures to the picked groups", () => {
    renderOverview({}, "/?groupBy=project&pick=search");
    expect(screen.getByText("$30.00")).toBeInTheDocument();
    expect(screen.getByTestId("chart")).not.toHaveTextContent("Payments");
  });

  it("shows loading skeletons and no chart while loading", () => {
    hoisted.state = { data: undefined, isLoading: true, error: undefined };
    renderOverview();
    expect(screen.queryByTestId("chart")).toBeNull();
    expect(screen.queryByText(/This month/)).toBeNull();
  });

  it("shows an error notice when cost cannot load", () => {
    hoisted.state = {
      data: undefined,
      isLoading: false,
      error: new Error("500"),
    };
    renderOverview();
    expect(screen.getByText("Couldn't load cost")).toBeInTheDocument();
    expect(screen.queryByTestId("chart")).toBeNull();
  });

  it("shows an empty state without traffic", () => {
    hoisted.state.data = { costByGroup: [] };
    renderOverview();
    expect(screen.getByText("No traffic yet")).toBeInTheDocument();
    expect(screen.queryByTestId("chart")).toBeNull();
  });

  it("scopes a project's page and groups by agent", () => {
    renderOverview({ projectName: "payments" });
    expect(hoisted.useCosts).toHaveBeenCalledWith(
      { orgName: "acme" },
      { groupBy: "agent", filter: ["project:payments"] },
    );
    expect(screen.getByText(/in payments/)).toBeInTheDocument();
  });

  it("scopes an agent's page and groups by model", () => {
    hoisted.state.data = { costByGroup: [group("gpt-4o", ["1", "2", "3"])] };
    renderOverview({ projectName: "payments", agentName: "checkout" });
    expect(hoisted.useCosts).toHaveBeenCalledWith(
      { orgName: "acme" },
      { groupBy: "model", filter: ["agent:payments/checkout"] },
    );
    expect(screen.getByText(/for agent checkout/)).toBeInTheDocument();
    expect(screen.getByTestId("chart")).toHaveTextContent("gpt-4o");
  });

  it("writes the group-by choice to the URL", () => {
    renderOverview();
    fireEvent.mouseDown(screen.getByText("Group by: Project"));
    fireEvent.click(
      within(screen.getByRole("listbox")).getByText("LLM provider"),
    );
    expect(screen.getByTestId("where")).toHaveTextContent(
      "?groupBy=llmProvider",
    );
    expect(hoisted.useCosts).toHaveBeenLastCalledWith(
      { orgName: "acme" },
      { groupBy: "llmProvider", filter: undefined },
    );
  });

  it("shows Advanced analytics only when the deployment names a dashboard", () => {
    const { unmount } = renderOverview();
    expect(screen.queryByText("Advanced analytics")).toBeNull();
    unmount();
    hoisted.config = {
      analyticsDashboardUrl: " https://app.moesif.com/wso2 ",
    };
    renderOverview();
    expect(
      screen.getByRole("link", { name: /Advanced analytics/ }),
    ).toHaveAttribute("href", "https://app.moesif.com/wso2");
  });
});

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

import { render, renderHook, screen } from "@testing-library/react";
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
  chart: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useCosts: (...args: unknown[]) => {
    hoisted.useCosts(...args);
    return hoisted.state;
  },
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  NESTED_CARD_SX: {},
}));
vi.mock("../AICostManagement/subComponents/CostChart", () => ({
  CostChart: (props: {
    rows: { month: string }[];
    series: { name: string }[];
    height?: number;
    maxBarSize?: number;
  }) => {
    hoisted.chart(props);
    return (
      <div data-testid="chart">
        {props.rows.map((r) => r.month).join("|")} /{" "}
        {props.series.map((s) => s.name).join("|")}
      </div>
    );
  },
}));

import {
  EntityCostChart,
  EntityCostTiles,
  useEntityCost,
} from "./EntityCostPanel";

const group = (
  id: string,
  costs: [string, string, string],
  project?: string,
): CostByGroup =>
  ({
    id,
    project,
    cost: String(costs.reduce((a, c) => a + Number(c), 0)),
    unpricedRequests: { missingPrice: 0, usageUnavailable: 0 },
    monthly: [
      { month: "2026-09", cost: costs[0] },
      { month: "2026-10", cost: costs[1] },
      { month: "2026-11", cost: costs[2] },
    ],
  }) as unknown as CostByGroup;

const wrap = (ui: React.ReactElement) =>
  render(<ThemeProvider theme={createTheme()}>{ui}</ThemeProvider>);

beforeEach(() => {
  hoisted.useCosts.mockClear();
  hoisted.chart.mockClear();
  hoisted.state = {
    data: {
      costByGroup: [
        group("gpt-4o", ["10", "20", "40"]),
        group("claude-sonnet", ["5", "10", "15"]),
      ],
    },
    isLoading: false,
    error: undefined,
  };
});

describe("useEntityCost", () => {
  it("reads the entity's cost with its filter and totals the kept months", () => {
    const { result } = renderHook(() =>
      useEntityCost("acme", "project:payments"),
    );
    expect(hoisted.useCosts).toHaveBeenCalledWith(
      { orgName: "acme" },
      { groupBy: "organization", filter: ["project:payments"] },
    );
    expect(result.current).toMatchObject({
      current: 55,
      last: 30,
      window: 100,
      thisMonth: "Nov",
      lastMonth: "Oct",
      firstMonth: "Sep",
      totals: [15, 30, 55],
    });
  });

  it("is zero without data", () => {
    hoisted.state.data = undefined;
    const { result } = renderHook(() => useEntityCost("acme", "agent:a/b"));
    expect(result.current.current).toBe(0);
    expect(result.current.window).toBe(0);
  });
});

describe("EntityCostTiles", () => {
  it("shows this month, last month and the three-month figures", () => {
    wrap(<EntityCostTiles orgName="acme" filter="project:payments" />);
    expect(screen.getByText("This month (Nov) so far")).toBeInTheDocument();
    expect(screen.getByText("$55.00")).toBeInTheDocument();
    expect(screen.getByText("Last month (Oct)")).toBeInTheDocument();
    expect(screen.getByText("$30.00")).toBeInTheDocument();
    expect(screen.getByText("Last 3 months (Sep–Nov)")).toBeInTheDocument();
    expect(screen.getByText("$100.00")).toBeInTheDocument();
  });

  it("stacks the tiles when asked", () => {
    wrap(<EntityCostTiles orgName="acme" filter="agent:p/a" stacked />);
    expect(screen.getByText("This month (Nov) so far")).toBeInTheDocument();
    expect(screen.getByText("Last month (Oct)")).toBeInTheDocument();
  });

  it("hides the amounts while loading", () => {
    hoisted.state = { data: undefined, isLoading: true, error: undefined };
    wrap(<EntityCostTiles orgName="acme" filter="agent:p/a" />);
    expect(screen.queryByText(/\$/)).toBeNull();
  });

  it("says cost could not be loaded on error", () => {
    hoisted.state = {
      data: undefined,
      isLoading: false,
      error: new Error("500"),
    };
    wrap(<EntityCostTiles orgName="acme" filter="agent:p/a" />);
    expect(
      screen.getByText("Couldn't load cost. Reload the page to try again."),
    ).toBeInTheDocument();
  });
});

describe("EntityCostChart", () => {
  it("charts the entity's cost by the chosen dimension", () => {
    wrap(
      <EntityCostChart
        orgName="acme"
        filter="agent:payments/checkout"
        groupBy="model"
      />,
    );
    expect(hoisted.useCosts).toHaveBeenCalledWith(
      { orgName: "acme" },
      { groupBy: "model", filter: ["agent:payments/checkout"] },
    );
    expect(screen.getByText("Monthly cost by model (USD)")).toBeInTheDocument();
    expect(screen.getByTestId("chart")).toHaveTextContent(
      "Sep|Oct|Nov (so far) / gpt-4o|claude-sonnet",
    );
    expect(hoisted.chart.mock.calls[0][0]).toMatchObject({ height: 260 });
  });

  it("titles an LLM provider grouping and uses display names", () => {
    hoisted.state.data = {
      costByGroup: [group("azure-openai", ["1", "2", "3"])],
    };
    wrap(
      <EntityCostChart
        orgName="acme"
        filter="project:payments"
        groupBy="llmProvider"
        names={{ llmProvider: { "azure-openai": "Azure OpenAI" } }}
        height={130}
        maxBarSize={120}
        bare
      />,
    );
    expect(
      screen.getByText(/Monthly cost by LLM provider/),
    ).toBeInTheDocument();
    expect(screen.getByTestId("chart")).toHaveTextContent("Azure OpenAI");
    expect(hoisted.chart.mock.calls[0][0]).toMatchObject({
      height: 130,
      maxBarSize: 120,
    });
  });

  it("sums groups beyond the top five as Other and notes free groups", () => {
    hoisted.state.data = {
      costByGroup: [
        ...["a", "b", "c", "d", "e", "f", "g"].map((id, i) =>
          group(id, ["1", "1", String(10 - i)]),
        ),
        group("ledger", ["0", "0", "0"]),
      ],
    };
    wrap(
      <EntityCostChart
        orgName="acme"
        filter="project:payments"
        groupBy="model"
        fill
      />,
    );
    expect(screen.getByTestId("chart")).toHaveTextContent("Other (2)");
    expect(
      screen.getByText("Not shown (charged $0): ledger."),
    ).toBeInTheDocument();
  });

  it("draws an empty chart over three months without cost", () => {
    hoisted.state.data = { costByGroup: [] };
    wrap(<EntityCostChart orgName="acme" filter="x:y" groupBy="model" />);
    const rows = hoisted.chart.mock.calls[0][0].rows;
    expect(rows).toHaveLength(3);
    expect(hoisted.chart.mock.calls[0][0].series).toEqual([]);
  });

  it("renders nothing on error and a skeleton while loading", () => {
    hoisted.state = {
      data: undefined,
      isLoading: false,
      error: new Error("500"),
    };
    const { container, unmount } = wrap(
      <EntityCostChart orgName="acme" filter="x:y" groupBy="model" />,
    );
    expect(container.querySelector(".MuiCard-root")).toBeNull();
    expect(screen.queryByTestId("chart")).toBeNull();
    unmount();
    hoisted.state = { data: undefined, isLoading: true, error: undefined };
    wrap(<EntityCostChart orgName="acme" filter="x:y" groupBy="model" />);
    expect(screen.queryByTestId("chart")).toBeNull();
    expect(document.querySelector(".MuiSkeleton-root")).not.toBeNull();
  });
});

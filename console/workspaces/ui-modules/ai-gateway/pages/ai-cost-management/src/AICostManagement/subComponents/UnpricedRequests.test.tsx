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

import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
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
  useEntityNames: () => ({ project: {}, agent: {}, llmProvider: {} }),
}));

import { UnpricedRequests } from "./UnpricedRequests";

const entry = (
  id: string | null,
  missingPrice: number,
  usageUnavailable: number,
): CostByGroup => ({
  id,
  cost: "12.50",
  unpricedRequests: { missingPrice, usageUnavailable },
  monthly: [
    { month: "2026-09", cost: "1.00" },
    { month: "2026-10", cost: "2.00" },
    { month: "2026-11", cost: "3.00" },
  ],
});

const renderPage = (
  props: { projectName?: string; agentName?: string } = {},
  url = "/",
) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={[url]}>
        <UnpricedRequests orgName="acme" backHref="/cost" {...props} />
      </MemoryRouter>
    </ThemeProvider>,
  );

beforeEach(() => {
  hoisted.useCosts.mockClear();
  hoisted.state.isLoading = false;
  hoisted.state.error = undefined;
  hoisted.state.data = { costByGroup: [] };
});

describe("UnpricedRequests", () => {
  it("shows the empty state when everything was priced", () => {
    renderPage();
    expect(screen.getByText("Requests charged $0")).toBeTruthy();
    expect(screen.getByText("No requests charged $0")).toBeTruthy();
    expect(
      screen.getByText("All requests in the last 3 months were priced."),
    ).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("shows skeletons while loading", () => {
    hoisted.state.isLoading = true;
    hoisted.state.data = undefined;
    renderPage();
    expect(screen.queryByText("Missing price")).toBeNull();
    expect(screen.queryByText("No requests charged $0")).toBeNull();
  });

  it("shows an error instead of the tiles", () => {
    hoisted.state.error = new Error("boom");
    renderPage();
    expect(
      screen.getByText("Couldn't load the requests charged $0."),
    ).toBeTruthy();
    expect(screen.queryByText("Missing price")).toBeNull();
  });

  it("totals the tiles and ranks models by affected requests", () => {
    hoisted.state.data = {
      costByGroup: [
        entry("claude-sonnet-4", 2, 1),
        entry("gpt-4o", 10, 5),
        entry("gpt-4o-mini", 0, 0),
        entry("gemini-2.0-flash", 3, 0),
        entry(null, 0, 3),
      ],
    };
    renderPage();
    expect(screen.getByText(/Last 3 months \(Sep–Nov\)/)).toBeTruthy();
    const tile = (caption: string) =>
      screen.getByText(caption).parentElement as HTMLElement;
    expect(within(tile("No price for the model")).getByText("15")).toBeTruthy();
    expect(
      within(tile("No token usage in the response")).getByText("9"),
    ).toBeTruthy();
    expect(within(tile("Last 3 months")).getByText("24")).toBeTruthy();

    const rows = screen.getAllByRole("row").slice(1);
    expect(
      rows.map((r) =>
        within(r)
          .getAllByRole("cell")
          .map((c) => c.textContent),
      ),
    ).toEqual([
      ["gpt-4o", "10", "5", "15"],
      // equal totals of 3 sort by model name
      ["claude-sonnet-4", "2", "1", "3"],
      ["gemini-2.0-flash", "3", "0", "3"],
      ["Unknown model", "0", "3", "3"],
    ]);
  });

  it("queries by model for an organization", () => {
    renderPage();
    expect(hoisted.useCosts).toHaveBeenCalledWith(
      { orgName: "acme" },
      { groupBy: "model", filter: undefined },
    );
  });

  it("scopes the query to a project and the picked values", () => {
    renderPage(
      { projectName: "payments" },
      "/?groupBy=agent&pick=payments/refund-bot",
    );
    expect(hoisted.useCosts).toHaveBeenLastCalledWith(
      { orgName: "acme" },
      {
        groupBy: "model",
        filter: ["project:payments", "agent:payments/refund-bot"],
      },
    );
  });

  it("links back to the dashboard", () => {
    renderPage();
    expect(
      screen
        .getByRole("link", { name: /back to dashboard/i })
        .getAttribute("href"),
    ).toBe("/cost");
  });
});

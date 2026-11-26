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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.setConfig({ testTimeout: 60_000 });

const hoisted = vi.hoisted(() => ({
  access: new Set<string>(),
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureManagement: () => ({
    hasAccess: (feature: string) => hoisted.access.has(feature),
  }),
}));
vi.mock("./EntityCostPanel", () => ({
  EntityCostTiles: (p: {
    orgName: string;
    filter: string;
    stacked?: boolean;
  }) => (
    <div data-testid="tiles">
      {p.orgName} {p.filter} {String(p.stacked)}
    </div>
  ),
  EntityCostChart: (p: {
    orgName: string;
    filter: string;
    groupBy: string;
    height: number;
  }) => (
    <div data-testid="chart">
      {p.orgName} {p.filter} {p.groupBy} {p.height}
    </div>
  ),
}));
vi.mock("./QuickBudgetDialog", () => ({
  QuickBudgetDialog: (p: {
    onClose: () => void;
    orgName: string;
    start: unknown;
    kind: string;
    label: string;
    createHref: string;
    viewHref: (n: string) => string;
  }) => (
    <div data-testid="quick">
      {p.orgName} {JSON.stringify(p.start)} {p.kind} {p.label} {p.createHref}{" "}
      {p.viewHref("azure-eu-monthly")}
      <button onClick={p.onClose}>close quick</button>
    </div>
  ),
}));

import { LLMProviderCostSection } from "./OverviewCostPanels";

const renderSection = (providerName?: string) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter>
        <LLMProviderCostSection
          orgId="acme"
          providerId="azure-openai"
          providerName={providerName}
        />
      </MemoryRouter>
    </ThemeProvider>,
  );

beforeEach(() => {
  hoisted.access = new Set(["cost_write"]);
});

describe("LLMProviderCostSection", () => {
  it("shows the provider's tiles and its cost by model", () => {
    renderSection("Azure OpenAI");
    expect(screen.getByText("Cost")).toBeInTheDocument();
    expect(screen.getByTestId("tiles")).toHaveTextContent(
      "acme llmProvider:azure-openai true",
    );
    expect(screen.getByTestId("chart")).toHaveTextContent(
      "acme llmProvider:azure-openai model 130",
    );
  });

  it("links to the Dashboard picked to the provider", () => {
    renderSection();
    expect(
      screen.getByRole("link", { name: /View cost details/ }),
    ).toHaveAttribute(
      "href",
      "/org/acme/cost-management?groupBy=llmProvider&pick=azure-openai",
    );
  });

  it("opens the quick budget form for the provider and closes it", () => {
    renderSection("Azure OpenAI");
    expect(screen.queryByTestId("quick")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Add budget" }));
    expect(screen.getByTestId("quick")).toHaveTextContent(
      'acme {"provider":"azure-openai"} LLM provider Azure OpenAI /org/acme/cost-management/budgets/create /org/acme/cost-management/budgets/view/azure-eu-monthly',
    );
    fireEvent.click(screen.getByText("close quick"));
    expect(screen.queryByTestId("quick")).toBeNull();
  });

  it("labels the budget with the handle when no name is given", () => {
    renderSection();
    fireEvent.click(screen.getByRole("button", { name: "Add budget" }));
    expect(screen.getByTestId("quick")).toHaveTextContent(
      "LLM provider azure-openai",
    );
  });

  it("hides Add budget without cost_write", () => {
    hoisted.access = new Set(["cost_read"]);
    renderSection();
    expect(screen.queryByRole("button", { name: "Add budget" })).toBeNull();
    expect(screen.getByTestId("tiles")).toBeInTheDocument();
  });
});

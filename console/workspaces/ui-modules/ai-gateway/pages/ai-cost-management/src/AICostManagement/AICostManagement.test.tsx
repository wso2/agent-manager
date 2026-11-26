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
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.setConfig({ testTimeout: 60_000 });

const hoisted = vi.hoisted(() => ({
  access: new Set<string>(),
  names: {
    project: { payments: "Payments" } as Record<string, string>,
    agent: { "payments/checkout": "Checkout Agent" } as Record<string, string>,
  },
  returnTo: vi.fn(),
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureManagement: () => ({
    hasAccess: (feature: string) => hoisted.access.has(feature),
  }),
}));
vi.mock("@agent-management-platform/views", () => ({
  PageLayout: ({
    title,
    description,
    actions,
    children,
  }: {
    title: string;
    description: React.ReactNode;
    actions?: React.ReactNode;
    children: React.ReactNode;
  }) => (
    <section>
      <h1>{title}</h1>
      <p data-testid="description">{description}</p>
      <div data-testid="actions">{actions}</div>
      {children}
    </section>
  ),
  returnToState: () => ({ returnTo: "/came-from" }),
  useReturnTo: (fallback: string) => {
    hoisted.returnTo(fallback);
    return fallback;
  },
}));
vi.mock("./useEntityNames", () => ({
  useEntityNames: () => hoisted.names,
}));
vi.mock("./subComponents/CostOverview", () => ({
  CostOverview: (p: Record<string, string | undefined>) => (
    <div data-testid="overview">
      {[p.orgName, p.projectName, p.agentName, p.unpricedHref].join(" ")}
    </div>
  ),
}));
vi.mock("./subComponents/BudgetsTab", () => ({
  BudgetCreateAction: ({
    projectName,
    onCreate,
  }: {
    projectName?: string;
    onCreate: () => void;
  }) => (
    <button onClick={onCreate}>
      {projectName ? "Add budget" : "Create budget"}
    </button>
  ),
  BudgetsTab: ({
    onOpen,
    onEmptyChange,
    createAction,
    agentName,
  }: {
    onOpen: (name: string) => void;
    onEmptyChange: (empty: boolean) => void;
    createAction: React.ReactNode;
    agentName?: string;
  }) => (
    <div data-testid="budgets">
      budgets tab {agentName}
      <button onClick={() => onOpen("azure-eu-monthly")}>open budget</button>
      <button onClick={() => onEmptyChange(true)}>report empty</button>
      {createAction}
    </div>
  ),
}));
vi.mock("./subComponents/BudgetDetail", () => ({
  BudgetDetail: (p: {
    budgetName: string;
    backHref: string;
    editHref: string;
    readOnly: boolean;
    unpricedHref: string;
  }) => (
    <div data-testid="detail">
      {p.budgetName} {p.backHref} {p.editHref} {String(p.readOnly)}{" "}
      {p.unpricedHref}
    </div>
  ),
}));
vi.mock("./subComponents/BudgetForm", () => ({
  BudgetWizard: (p: {
    budgetName?: string;
    backHref: string;
    backLabel: string;
    onSaved: (name: string) => void;
  }) => (
    <div data-testid="wizard">
      {p.budgetName ?? "new"} {p.backHref} {p.backLabel}
      <button onClick={() => p.onSaved("anthropic-quarterly")}>saved</button>
    </div>
  ),
}));
vi.mock("./subComponents/UnpricedRequests", () => ({
  UnpricedRequests: (p: Record<string, string | undefined>) => (
    <div data-testid="unpriced">
      {p.orgName} {p.projectName} {p.agentName} {p.backHref}
    </div>
  ),
}));
vi.mock("../panels/QuickBudgetDialog", () => ({
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
vi.mock("../Pricing/PricingList", () => ({
  PricingList: ({ orgName }: { orgName: string }) => (
    <div data-testid="pricing">{orgName}</div>
  ),
}));

import { AICostManagement } from "./AICostManagement";

const Where = () => {
  const l = useLocation();
  return (
    <span data-testid="where">
      {l.pathname}
      {l.search}
    </span>
  );
};

const ORG = "/org/acme/cost-management";
const PROJECT = "/org/acme/project/payments/cost-management";
const AGENT = "/org/acme/project/payments/agents/checkout/cost-management";

const renderAt = (url: string) =>
  render(
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={[url]}>
        <Routes>
          <Route
            path="/org/:orgId/project/:projectId/agents/:agentId/cost-management/*"
            element={<AICostManagement />}
          />
          <Route
            path="/org/:orgId/project/:projectId/cost-management/*"
            element={<AICostManagement />}
          />
          <Route
            path="/org/:orgId/cost-management/*"
            element={<AICostManagement />}
          />
        </Routes>
        <Where />
      </MemoryRouter>
    </ThemeProvider>,
  );

const where = () => screen.getByTestId("where").textContent;

beforeEach(() => {
  hoisted.access = new Set(["cost_write"]);
  hoisted.returnTo.mockClear();
  hoisted.names = {
    project: { payments: "Payments" },
    agent: { "payments/checkout": "Checkout Agent" },
  };
});

describe("AICostManagement", () => {
  it("shows the organization Dashboard at the index", () => {
    renderAt(ORG);
    expect(screen.getByRole("heading", { name: "Dashboard" })).toBeVisible();
    expect(screen.getByTestId("description")).toHaveTextContent(
      "Monthly LLM cost of the organization.",
    );
    expect(screen.getByTestId("overview")).toHaveTextContent(
      `acme ${ORG}/unpriced-requests`,
    );
  });

  it("describes a project's Dashboard by its display name", () => {
    renderAt(PROJECT);
    expect(screen.getByTestId("description")).toHaveTextContent(
      "Monthly LLM cost of project Payments.",
    );
    expect(screen.getByTestId("overview")).toHaveTextContent("acme payments");
  });

  it("describes an agent's Dashboard by its display name", () => {
    renderAt(AGENT);
    expect(screen.getByTestId("description")).toHaveTextContent(
      "Monthly LLM cost of agent Checkout Agent.",
    );
    expect(screen.getByTestId("overview")).toHaveTextContent(
      "acme payments checkout",
    );
  });

  it("falls back to handles when names are unknown", () => {
    hoisted.names = { project: {}, agent: {} };
    renderAt(PROJECT);
    expect(screen.getByTestId("description")).toHaveTextContent(
      "Monthly LLM cost of project payments.",
    );
  });

  it("shows the Budgets tab with the create action in the header", () => {
    renderAt(`${ORG}/budgets`);
    expect(screen.getByRole("heading", { name: "Budgets" })).toBeVisible();
    expect(screen.getByTestId("description")).toHaveTextContent(
      "Spend limits for LLM traffic.",
    );
    expect(screen.getByTestId("budgets")).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: "Create budget" }),
    ).toHaveLength(2);
  });

  it("moves the create action out of the header when the list is empty", () => {
    renderAt(`${ORG}/budgets`);
    fireEvent.click(screen.getByText("report empty"));
    expect(
      screen.getAllByRole("button", { name: "Create budget" }),
    ).toHaveLength(1);
  });

  it("describes project and agent budgets", () => {
    const { unmount } = renderAt(`${PROJECT}/budgets`);
    expect(screen.getByTestId("description")).toHaveTextContent(
      "Budgets for project Payments.",
    );
    unmount();
    renderAt(`${AGENT}/budgets`);
    expect(screen.getByTestId("description")).toHaveTextContent(
      "Budgets for agent Checkout Agent.",
    );
  });

  it("opens a budget from the list", () => {
    renderAt(`${ORG}/budgets`);
    fireEvent.click(screen.getByText("open budget"));
    expect(where()).toBe(`${ORG}/budgets/view/azure-eu-monthly`);
    expect(screen.getByTestId("detail")).toHaveTextContent(
      `azure-eu-monthly ${ORG}/budgets ${ORG}/budgets/view/azure-eu-monthly/edit false ${ORG}/unpriced-requests`,
    );
  });

  it("navigates to the create form from the organization's page", () => {
    renderAt(`${ORG}/budgets`);
    fireEvent.click(
      screen.getAllByRole("button", { name: "Create budget" })[0],
    );
    expect(where()).toBe(`${ORG}/budgets/create`);
    expect(screen.getByTestId("wizard")).toHaveTextContent(
      `new ${ORG}/budgets Back to budgets`,
    );
  });

  it("opens the quick form on a project's page, filled in with the project", () => {
    renderAt(`${PROJECT}/budgets`);
    fireEvent.click(screen.getAllByRole("button", { name: "Add budget" })[0]);
    expect(screen.getByTestId("quick")).toHaveTextContent(
      `acme {"project":"payments"} Project Payments ${ORG}/budgets/create ${ORG}/budgets/view/azure-eu-monthly`,
    );
    fireEvent.click(screen.getByText("close quick"));
    expect(screen.queryByTestId("quick")).toBeNull();
  });

  it("opens the quick form on an agent's page as an Agent budget", () => {
    renderAt(`${AGENT}/budgets`);
    fireEvent.click(screen.getAllByRole("button", { name: "Add budget" })[0]);
    expect(screen.getByTestId("quick")).toHaveTextContent(
      '{"agent":"payments/checkout"} Agent Checkout Agent',
    );
  });

  it("shows a project's budget read-only", () => {
    renderAt(`${PROJECT}/budgets/view/azure-eu-monthly`);
    expect(screen.getByTestId("detail")).toHaveTextContent(
      `azure-eu-monthly ${PROJECT}/budgets ${ORG}/budgets/view/azure-eu-monthly/edit true`,
    );
  });

  it("shows the unpriced requests page with a way back keeping the selection", () => {
    renderAt(`${ORG}/unpriced-requests?groupBy=model&pick=gpt-4o`);
    expect(screen.getByTestId("unpriced")).toHaveTextContent(
      `acme ${ORG}?groupBy=model&pick=gpt-4o`,
    );
  });

  it("goes back to the Dashboard without a selection", () => {
    renderAt(`${AGENT}/unpriced-requests`);
    expect(screen.getByTestId("unpriced")).toHaveTextContent(
      `acme payments checkout ${AGENT}`,
    );
  });

  it("shows the pricing catalog at organization level", () => {
    renderAt(`${ORG}/pricing`);
    expect(
      screen.getByRole("heading", { name: "Pricing catalog" }),
    ).toBeVisible();
    expect(screen.getByTestId("pricing")).toHaveTextContent("acme");
  });

  it("redirects a project's pricing page to its Dashboard", () => {
    renderAt(`${PROJECT}/pricing`);
    expect(where()).toBe(PROJECT);
    expect(screen.getByTestId("overview")).toBeInTheDocument();
  });

  it("sends users without cost_write from the create form to the list", () => {
    hoisted.access = new Set();
    renderAt(`${ORG}/budgets/create`);
    expect(where()).toBe(`${ORG}/budgets`);
    expect(screen.getByTestId("budgets")).toBeInTheDocument();
  });

  it("sends a project's create form to its list even with cost_write", () => {
    renderAt(`${PROJECT}/budgets/create`);
    expect(where()).toBe(`${PROJECT}/budgets`);
  });

  it("edits a budget with cost_write and returns to the budget on save", () => {
    renderAt(`${ORG}/budgets/view/azure-eu-monthly/edit`);
    expect(screen.getByTestId("wizard")).toHaveTextContent(
      `azure-eu-monthly ${ORG}/budgets/view/azure-eu-monthly Back to the budget`,
    );
    expect(hoisted.returnTo).toHaveBeenCalledWith(
      `${ORG}/budgets/view/azure-eu-monthly`,
    );
    fireEvent.click(screen.getByText("saved"));
    expect(where()).toBe(`${ORG}/budgets/view/anthropic-quarterly`);
  });

  it("sends users without cost_write from edit to the budget's page", () => {
    hoisted.access = new Set();
    renderAt(`${ORG}/budgets/view/azure-eu-monthly/edit`);
    expect(where()).toBe(`${ORG}/budgets/view/azure-eu-monthly`);
    expect(screen.getByTestId("detail")).toBeInTheDocument();
  });

  it("redirects unknown paths to the Dashboard", () => {
    renderAt(`${ORG}/nowhere`);
    expect(where()).toBe(ORG);
    expect(screen.getByTestId("overview")).toBeInTheDocument();
  });
});

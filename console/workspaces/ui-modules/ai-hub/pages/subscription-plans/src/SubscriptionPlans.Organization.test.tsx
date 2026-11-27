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

import type { ReactNode } from "react";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation, useParams } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SubscriptionPlansOrganization } from "./SubscriptionPlans.Organization";

const state = vi.hoisted(() => ({
  access: {} as Record<string, boolean>,
  total: 0 as number | undefined,
  listArgs: [] as unknown[],
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: (key: string) => state.access[key] ?? false,
}));
vi.mock("@agent-management-platform/api-client", () => ({
  useListSubscriptionPlans: (...args: unknown[]) => {
    state.listArgs = args;
    return {
      data: state.total === undefined ? undefined : { pagination: { total: state.total } },
    };
  },
}));
vi.mock("@agent-management-platform/views", () => ({
  PageLayout: ({
    title,
    actions,
    children,
  }: {
    title: string;
    actions?: ReactNode;
    children: ReactNode;
  }) => (
    <div>
      <h1>{title}</h1>
      <div data-testid="header-actions">{actions}</div>
      {children}
    </div>
  ),
}));
vi.mock("./subComponents/SubscriptionPlanTable", () => ({
  SubscriptionPlanTable: ({ createAction }: { createAction?: ReactNode }) => (
    <div data-testid="plan-table">{createAction}</div>
  ),
}));
vi.mock("./subComponents/SubscriptionPlanFormPage", () => ({
  SubscriptionPlanFormPage: () => {
    const { planId } = useParams();
    return <p>{planId ? `Form for ${planId}` : "Form for a new plan"}</p>;
  },
}));

function Where() {
  return <p data-testid="location">{useLocation().pathname}</p>;
}

const renderAt = (path: string) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route
          element={
            <>
              <SubscriptionPlansOrganization />
              <Where />
            </>
          }
          path="/org/:orgId/subscription-plans/*"
        />
      </Routes>
    </MemoryRouter>,
  );

const LIST = "/org/acme/subscription-plans";
const createLinks = () => screen.queryAllByRole("link", { name: "Create plan" });

beforeEach(() => {
  state.access = {
    subscription_plan_create: true,
    subscription_plan_update: true,
  };
  state.total = 0;
  state.listArgs = [];
});

describe("SubscriptionPlansOrganization", () => {
  it("lists the org's plans, asking for the table's first page", () => {
    renderAt(LIST);
    expect(screen.getByRole("heading", { name: "Subscription Plans" })).toBeInTheDocument();
    expect(screen.getByTestId("plan-table")).toBeInTheDocument();
    expect(state.listArgs[0]).toEqual({ orgName: "acme" });
    expect(state.listArgs[1]).toEqual(expect.objectContaining({ offset: 0 }));
  });

  it("offers Create plan in the empty table only while the org has no plans", () => {
    renderAt(LIST);
    expect(createLinks()).toHaveLength(1);
    expect(screen.getByTestId("header-actions")).toBeEmptyDOMElement();
    expect(createLinks()[0]).toHaveAttribute("href", `${LIST}/add`);
  });

  it("also offers Create plan in the header once the org has plans", () => {
    state.total = 3;
    renderAt(LIST);
    expect(createLinks()).toHaveLength(2);
    expect(screen.getByTestId("header-actions")).not.toBeEmptyDOMElement();
  });

  it("treats a list not loaded yet as empty", () => {
    state.total = undefined;
    renderAt(LIST);
    expect(screen.getByTestId("header-actions")).toBeEmptyDOMElement();
  });

  it("hides Create plan without the create permission", () => {
    state.total = 3;
    state.access = {};
    renderAt(LIST);
    expect(createLinks()).toHaveLength(0);
  });

  it("routes add to the plan form", () => {
    renderAt(`${LIST}/add`);
    expect(screen.getByText("Form for a new plan")).toBeInTheDocument();
  });

  it("routes edit to the plan form with the plan's id", () => {
    renderAt(`${LIST}/edit/gold`);
    expect(screen.getByText("Form for gold")).toBeInTheDocument();
  });

  it("sends add back to the list without the create permission", () => {
    state.access = { subscription_plan_update: true };
    renderAt(`${LIST}/add`);
    expect(screen.queryByText("Form for a new plan")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(LIST);
    expect(screen.getByTestId("plan-table")).toBeInTheDocument();
  });

  it("sends edit back to the list without the update permission", () => {
    state.access = { subscription_plan_create: true };
    renderAt(`${LIST}/edit/gold`);
    expect(screen.queryByText("Form for gold")).not.toBeInTheDocument();
    expect(screen.getByTestId("location").textContent).toBe(LIST);
  });

  it("sends an unknown path back to the list", () => {
    renderAt(`${LIST}/nope`);
    expect(screen.getByTestId("location").textContent).toBe(LIST);
  });
});

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */


import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation, useParams } from "react-router-dom";
import {
  FeatureManagementProvider,
  type FeatureAccessConfig,
  type FeatureFlags,
} from "@agent-management-platform/feature-management";
import { GatewaysOrganization } from "./Gateways.Organization";

const hoisted = vi.hoisted(() => ({
  gateways: [] as Array<{ uuid: string }>,
}));

// MUI renders are slow on a busy machine; leave headroom over the 5s default.
vi.setConfig({ testTimeout: 30_000 });

vi.mock("@agent-management-platform/api-client", () => ({
  useListGateways: () => ({ data: { gateways: hoisted.gateways } }),
}));

vi.mock("@agent-management-platform/views", () => ({
  PageLayout: ({
    title,
    actions,
    children,
  }: {
    title: string;
    actions?: React.ReactNode;
    children: React.ReactNode;
  }) => (
    <div>
      <h1>{title}</h1>
      {actions}
      {children}
    </div>
  ),
  returnToState: (location: { pathname: string; search: string }) => ({
    from: `${location.pathname}${location.search}`,
  }),
}));

vi.mock("./subComponents/AIGatewaysTable", () => ({
  AIGatewaysTable: ({ onAddGateway }: { onAddGateway?: () => void }) => (
    <div>
      gateways table
      {onAddGateway && <button onClick={onAddGateway}>empty state add</button>}
    </div>
  ),
}));

vi.mock("./subComponents/ViewGateway", () => ({
  ViewGateway: () => <div>view gateway {useParams().gatewayId}</div>,
}));

vi.mock("./subComponents/EditGateway", () => ({
  EditGateway: () => <div>edit gateway {useParams().gatewayId}</div>,
}));

vi.mock("./subComponents/AddGateway", () => ({
  AddGateway: () => (
    <div>
      add gateway for {useParams().orgId} from{" "}
      {(useLocation().state as { from?: string } | null)?.from ?? "nowhere"}
    </div>
  ),
}));

vi.mock("./subComponents/SetupGateway", () => ({
  SetupGateway: () => <div>setup gateway {useParams().gatewayId}</div>,
}));

const Location = () => <div data-testid="location">{useLocation().pathname}</div>;

const renderAt = (
  path: string,
  {
    flags = { enableSelfHostedGateways: true },
    access = {},
  }: { flags?: Partial<FeatureFlags>; access?: Partial<FeatureAccessConfig> } = {},
) =>
  render(
    <FeatureManagementProvider featureFlags={flags} featureAccess={access}>
      <MemoryRouter
        initialEntries={[path]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/org/:orgId/gateways/*" element={<GatewaysOrganization />} />
          <Route path="*" element={null} />
        </Routes>
        <Location />
      </MemoryRouter>
    </FeatureManagementProvider>,
  );

beforeEach(() => {
  hoisted.gateways = [{ uuid: "gw-1" }];
});

describe("GatewaysOrganization", () => {
  it("lists gateways and opens the add page from the header", () => {
    renderAt("/org/acme/gateways");

    expect(screen.getByRole("heading", { name: "Gateways" })).toBeInTheDocument();
    expect(screen.getByText("gateways table")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Add gateway" }));
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/org\/acme\/gateways\/add$/);
    // The add page can return to the list that opened it.
    expect(screen.getByText("add gateway for acme from /org/acme/gateways")).toBeInTheDocument();
  });

  it("leaves adding to the table's empty state when there are no gateways", () => {
    hoisted.gateways = [];
    renderAt("/org/acme/gateways");

    expect(screen.queryByRole("button", { name: "Add gateway" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "empty state add" }));
    expect(screen.getByText("add gateway for acme from /org/acme/gateways")).toBeInTheDocument();
  });

  it("hides Add gateway when self-hosted gateways are off", () => {
    renderAt("/org/acme/gateways", { flags: { enableSelfHostedGateways: false } });

    expect(screen.getByText("gateways table")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Add gateway" })).not.toBeInTheDocument();
  });

  it("offers no way to add or edit without gateway write access", () => {
    renderAt("/org/acme/gateways/edit/gw-1", { access: { gateway_write: false } });

    expect(screen.queryByText("edit gateway gw-1")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/org\/acme\/gateways$/);
    expect(screen.queryByRole("button", { name: "Add gateway" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "empty state add" })).not.toBeInTheDocument();
  });

  it("keeps a user who cannot create tokens off the add page", () => {
    renderAt("/org/acme/gateways/add", { access: { gateway_token_write: false } });
    expect(screen.queryByText(/add gateway for/)).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/org\/acme\/gateways$/);
  });

  it("routes to the setup page for a writer", () => {
    renderAt("/org/acme/gateways/setup/gw-3");
    expect(screen.getByText("setup gateway gw-3")).toBeInTheDocument();
  });

  it("routes to the view page", () => {
    renderAt("/org/acme/gateways/view/gw-1");
    expect(screen.getByText("view gateway gw-1")).toBeInTheDocument();
  });

  it("routes to the edit page for a writer", () => {
    renderAt("/org/acme/gateways/edit/gw-2");
    expect(screen.getByText("edit gateway gw-2")).toBeInTheDocument();
  });

  it("redirects an unknown path to the gateway list", () => {
    renderAt("/org/acme/gateways/nope");
    expect(screen.getByTestId("location")).toHaveTextContent(/^\/org\/acme\/gateways$/);
    expect(screen.getByText("gateways table")).toBeInTheDocument();
  });
});

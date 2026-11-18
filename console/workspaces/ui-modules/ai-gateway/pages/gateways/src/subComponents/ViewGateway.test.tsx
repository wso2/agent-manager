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

import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import {
  FeatureManagementProvider,
  type FeatureFlags,
} from "@agent-management-platform/feature-management";
import { ViewGateway } from "./ViewGateway";

const getGateway = vi.hoisted(() => vi.fn());

vi.mock("@agent-management-platform/api-client", () => ({
  useGetGateway: getGateway,
  isGatewayActive: (gw?: { status?: string }) => gw?.status === "ACTIVE",
}));

vi.mock("@agent-management-platform/views", async () => {
  const React = await import("react");
  return {
    PageLayout: ({
      children,
      title,
      description,
      actions,
      titleTail,
    }: {
      children: React.ReactNode;
      title: string;
      description?: string;
      actions?: React.ReactNode;
      titleTail?: React.ReactNode;
    }) =>
      React.createElement(
        "div",
        null,
        React.createElement("h1", null, title),
        description && React.createElement("p", null, description),
        actions,
        titleTail,
        children,
      ),
  };
});

vi.mock("@agent-management-platform/shared-component", () => ({
  GatewayTypeChip: ({ type }: { type: string }) => <span>type {type}</span>,
  getErrorMessage: (e: unknown) => (e instanceof Error ? e.message : ""),
  formatRelativeTime: () => "3 days ago",
}));

vi.mock("./GatewayIdentityProvidersCard", () => ({
  GatewayIdentityProvidersCard: ({ configMode }: { configMode: string }) => (
    <div>
      {configMode === "manual" ? "self-hosted identity providers card" : "identity providers card"}
    </div>
  ),
}));

vi.mock("./SelfHostedGatewayTokensCard", () => ({
  SelfHostedGatewayTokensCard: () => <div>tokens card</div>,
}));

vi.mock("./SelfHostedSetupSteps", () => ({
  SelfHostedSetupSteps: () => <div>setup steps</div>,
}));

let featureFlags: Partial<FeatureFlags> = {};
let canWrite = true;

const Location = () => <div data-testid="location">{useLocation().pathname}</div>;

type GatewayQuery = { data?: Record<string, unknown>; isLoading?: boolean; error?: unknown };

const renderGateway = (
  hosting: "MANAGED" | "SELF_HOSTED",
  statusOrQuery: "ACTIVE" | "INACTIVE" | GatewayQuery = "ACTIVE",
) => {
  getGateway.mockReturnValue(
    typeof statusOrQuery === "object"
      ? { isLoading: false, error: null, ...statusOrQuery }
      : {
          data: {
            uuid: "gw-1",
            name: "gw-1",
            displayName: "Gateway 1",
            hosting,
            status: statusOrQuery,
            environments: [],
          },
          isLoading: false,
          error: null,
        },
  );
  render(
    <FeatureManagementProvider
      featureFlags={featureFlags}
      featureAccess={{ gateway_write: canWrite }}
    >
      <MemoryRouter
        initialEntries={["/org/acme/gateways/gw-1"]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/org/:orgId/gateways/:gatewayId" element={<ViewGateway />} />
          <Route path="*" element={null} />
        </Routes>
        <Location />
      </MemoryRouter>
    </FeatureManagementProvider>,
  );
};

afterEach(() => {
  featureFlags = {};
  canWrite = true;
});

describe("ViewGateway with enableSelfHostedGateways off", () => {
  it("shows a self-hosted gateway's setup steps and tokens until it connects", () => {
    featureFlags = { enableSelfHostedGateways: false };
    renderGateway("SELF_HOSTED", "INACTIVE");

    expect(screen.getByText("setup steps")).toBeInTheDocument();
    expect(screen.getByText("tokens card")).toBeInTheDocument();
    // A self-hosted gateway registers identity providers too, by hand in its config.toml.
    expect(screen.getByText("self-hosted identity providers card")).toBeInTheDocument();
  });

  it("hides the setup steps of a connected self-hosted gateway, keeping its tokens", () => {
    featureFlags = { enableSelfHostedGateways: false };
    renderGateway("SELF_HOSTED", "ACTIVE");

    expect(screen.queryByText("setup steps")).not.toBeInTheDocument();
    expect(screen.getByText("tokens card")).toBeInTheDocument();
  });

  it("shows neither the virtual host nor the environments", () => {
    renderGateway("MANAGED");

    expect(screen.queryByText("Virtual Host")).not.toBeInTheDocument();
    expect(screen.queryByText("Environments")).not.toBeInTheDocument();
  });

  it("shows a managed gateway's identity providers", () => {
    featureFlags = { enableSelfHostedGateways: false };
    renderGateway("MANAGED");

    expect(screen.getByText("identity providers card")).toBeInTheDocument();
    expect(screen.queryByText("setup steps")).not.toBeInTheDocument();
    expect(screen.queryByText("tokens card")).not.toBeInTheDocument();
  });
});

describe("ViewGateway details", () => {
  const detailed = {
    uuid: "gw-1",
    name: "edge",
    displayName: "Edge Gateway",
    hosting: "MANAGED",
    status: "INACTIVE",
    isCritical: true,
    gatewayType: "AI",
    vhost: "edge.example.com",
    createdAt: "2026-01-01T00:00:00Z",
    environments: [
      { id: "e1", name: "dev", displayName: "Development" },
      { id: "e2", name: "prod", displayName: "" },
    ],
  };

  it("shows the gateway's status", () => {
    renderGateway("MANAGED", { data: detailed });

    expect(screen.getByRole("heading", { name: "Edge Gateway" })).toBeInTheDocument();
    expect(screen.getByText("Created 3 days ago")).toBeInTheDocument();
    expect(screen.getByText("Inactive")).toBeInTheDocument();
    expect(screen.getByText("Critical")).toBeInTheDocument();
    expect(screen.getByText("type AI")).toBeInTheDocument();
    // The virtual host and environments are no longer shown on this page.
    expect(screen.queryByText("edge.example.com")).not.toBeInTheDocument();
  });

  it("opens the edit page of a self-hosted gateway", () => {
    renderGateway("SELF_HOSTED", { data: { ...detailed, hosting: "SELF_HOSTED" } });
    fireEvent.click(screen.getByRole("button", { name: "Edit gateway" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/org/acme/gateways/edit/gw-1");
  });

  it("offers no edit for a managed gateway: it is read-only", () => {
    renderGateway("MANAGED", { data: detailed });
    expect(screen.queryByRole("button", { name: "Edit gateway" })).not.toBeInTheDocument();
  });

  it("offers no edit without gateway write access", () => {
    canWrite = false;
    renderGateway("MANAGED", { data: detailed });
    expect(screen.queryByRole("button", { name: "Edit gateway" })).not.toBeInTheDocument();
  });

  it("reports a failed load and shows no details", () => {
    renderGateway("MANAGED", { data: undefined, error: new Error("gateway not found") });

    expect(screen.getByText(/Failed to load gateway\. gateway not found/)).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "gw-1" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Edit gateway" })).not.toBeInTheDocument();
    expect(screen.queryByText("identity providers card")).not.toBeInTheDocument();
  });

  it("shows no details while loading", () => {
    renderGateway("MANAGED", { data: undefined, isLoading: true });
    expect(screen.queryByText("Virtual Host")).not.toBeInTheDocument();
    expect(screen.queryByText(/Failed to load gateway/)).not.toBeInTheDocument();
  });
});

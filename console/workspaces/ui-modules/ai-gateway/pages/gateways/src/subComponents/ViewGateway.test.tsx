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
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import {
  FeatureManagementProvider,
  type FeatureFlags,
} from "@agent-management-platform/feature-management";
import { ViewGateway } from "./ViewGateway";
import { keepNewGatewayToken, newGatewayToken } from "./newGatewayToken";

const getGateway = vi.hoisted(() => vi.fn());

vi.mock("@agent-management-platform/api-client", () => ({
  useGetGateway: getGateway,
  isGatewayActive: (gw?: { status?: string }) => gw?.status === "ACTIVE",
}));

vi.mock("@agent-management-platform/views", async () => {
  const React = await import("react");
  return {
    PageLayout: ({ children }: { children: React.ReactNode }) =>
      React.createElement("div", null, children),
  };
});

vi.mock("@agent-management-platform/shared-component", () => ({
  GatewayTypeChip: () => null,
  getErrorMessage: () => "",
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

// Like the real steps, the stand-in reads the token from the store.
vi.mock("./SelfHostedSetupSteps", async () => {
  const { useNewGatewayToken } = await import("./newGatewayToken");
  return {
    SelfHostedSetupSteps: ({ gatewayId }: { gatewayId: string }) => (
      <div>
        <span>setup steps</span>
        <span>token={useNewGatewayToken(gatewayId) ?? "none"}</span>
      </div>
    ),
  };
});

let featureFlags: Partial<FeatureFlags> = {};

const renderGateway = (
  hosting: "MANAGED" | "SELF_HOSTED",
  status: "ACTIVE" | "INACTIVE" = "ACTIVE",
) => {
  getGateway.mockReturnValue({
    data: {
      uuid: "gw-1",
      name: "gw-1",
      displayName: "Gateway 1",
      hosting,
      status,
      environments: [],
    },
    isLoading: false,
    error: null,
  });
  render(
    <FeatureManagementProvider featureFlags={featureFlags}>
      <MemoryRouter
        initialEntries={["/org/acme/gateways/gw-1"]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/org/:orgId/gateways/:gatewayId" element={<ViewGateway />} />
        </Routes>
      </MemoryRouter>
    </FeatureManagementProvider>,
  );
};

afterEach(() => {
  featureFlags = {};
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

  it("shows the steps a token the add page left behind, and forgets it on leaving", async () => {
    keepNewGatewayToken("gw-1", "secret");
    renderGateway("SELF_HOSTED", "INACTIVE");

    expect(screen.getByText("token=secret")).toBeInTheDocument();
    cleanup();
    await waitFor(() => expect(newGatewayToken("gw-1")).toBeNull());
  });

  it("shows the steps a token that arrives after the page opened", () => {
    renderGateway("SELF_HOSTED", "INACTIVE");
    expect(screen.getByText("token=none")).toBeInTheDocument();

    act(() => keepNewGatewayToken("gw-1", "late"));

    expect(screen.getByText("token=late")).toBeInTheDocument();
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

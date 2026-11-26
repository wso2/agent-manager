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
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import {
  FeatureManagementProvider,
  type FeatureFlags,
} from "@agent-management-platform/feature-management";
import { ViewGateway } from "./ViewGateway";

const getGateway = vi.hoisted(() => vi.fn());
const mountedModules = vi.hoisted(() => vi.fn<(mountPoint: string) => unknown[]>(() => []));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetGateway: getGateway,
}));

vi.mock("@agent-management-platform/views", async () => {
  const React = await import("react");
  return {
    PageLayout: ({ children }: { children: React.ReactNode }) =>
      React.createElement("div", null, children),
    useExternalComponentModulesByMountPoint: mountedModules,
  };
});

vi.mock("@agent-management-platform/shared-component", () => ({
  GatewayTypeChip: () => null,
  getErrorMessage: () => "",
}));

vi.mock("./GatewayIdentityProvidersCard", () => ({
  GatewayIdentityProvidersCard: () => <div>identity providers card</div>,
}));

vi.mock("./SelfHostedGatewayTokensCard", () => ({
  SelfHostedGatewayTokensCard: () => <div>tokens card</div>,
}));

vi.mock("./SelfHostedSetupSteps", () => ({
  SelfHostedSetupSteps: () => <div>setup steps</div>,
}));

let featureFlags: Partial<FeatureFlags> = {};

const renderGateway = (hosting: "MANAGED" | "SELF_HOSTED") => {
  getGateway.mockReturnValue({
    data: {
      uuid: "gw-1",
      name: "gw-1",
      displayName: "Gateway 1",
      hosting,
      status: "ACTIVE",
      environments: [],
    },
    isLoading: false,
    error: null,
  });
  render(
    <FeatureManagementProvider featureFlags={featureFlags}>
      <MemoryRouter
        initialEntries={["/organizations/acme/gateways/gw-1"]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route path="/organizations/:orgId/gateways/:gatewayId" element={<ViewGateway />} />
        </Routes>
      </MemoryRouter>
    </FeatureManagementProvider>,
  );
};

afterEach(() => {
  featureFlags = {};
  mountedModules.mockImplementation(() => []);
});

const injectedSection = ({ gatewayId, orgId }: { gatewayId: string; orgId: string }) => (
  <div>
    injected section {orgId}/{gatewayId}
  </div>
);

describe("ViewGateway with enableSelfHostedGateways off", () => {
  it("shows a self-hosted gateway's setup steps and tokens", () => {
    featureFlags = { enableSelfHostedGateways: false };
    renderGateway("SELF_HOSTED");

    expect(screen.getByText("setup steps")).toBeInTheDocument();
    expect(screen.getByText("tokens card")).toBeInTheDocument();
    expect(screen.queryByText("identity providers card")).not.toBeInTheDocument();
  });

  it("shows a managed gateway's identity providers", () => {
    featureFlags = { enableSelfHostedGateways: false };
    renderGateway("MANAGED");

    expect(screen.getByText("identity providers card")).toBeInTheDocument();
    expect(screen.queryByText("setup steps")).not.toBeInTheDocument();
    expect(screen.queryByText("tokens card")).not.toBeInTheDocument();
  });
});

describe("ViewGateway gateway-details-section mount point", () => {
  it("renders an injected section on a managed gateway", () => {
    mountedModules.mockImplementation((mountPoint: string) =>
      mountPoint === "gateway-details-section"
        ? [{ kind: "component", moduleName: "Policy Configuration", component: injectedSection }]
        : [],
    );
    renderGateway("MANAGED");

    expect(screen.getByText("injected section acme/gw-1")).toBeInTheDocument();
  });

  it("does not render an injected section on a self-hosted gateway", () => {
    featureFlags = { enableSelfHostedGateways: false };
    mountedModules.mockReturnValue([
      { kind: "component", moduleName: "Policy Configuration", component: injectedSection },
    ]);
    renderGateway("SELF_HOSTED");

    expect(screen.queryByText(/injected section/)).not.toBeInTheDocument();
  });
});

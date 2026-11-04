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
import { globalConfig } from "@agent-management-platform/types";
import { ViewGateway } from "./ViewGateway";

const getGateway = vi.hoisted(() => vi.fn());

vi.mock("@agent-management-platform/api-client", () => ({
  useGetGateway: getGateway,
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
  GatewayIdentityProvidersCard: () => <div>identity providers card</div>,
}));

vi.mock("./SelfHostedGatewayTokensCard", () => ({
  SelfHostedGatewayTokensCard: () => <div>tokens card</div>,
}));

vi.mock("./SelfHostedSetupSteps", () => ({
  SelfHostedSetupSteps: () => <div>setup steps</div>,
}));

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
    <MemoryRouter
      initialEntries={["/org/acme/gateways/gw-1"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/gateways/:gatewayId" element={<ViewGateway />} />
      </Routes>
    </MemoryRouter>,
  );
};

afterEach(() => {
  delete globalConfig.featureFlags?.enableSelfHostedGateways;
});

describe("ViewGateway with enableSelfHostedGateways off", () => {
  it("shows a self-hosted gateway's setup steps and tokens", () => {
    globalConfig.featureFlags = { ...globalConfig.featureFlags, enableSelfHostedGateways: false };
    renderGateway("SELF_HOSTED");

    expect(screen.getByText("setup steps")).toBeInTheDocument();
    expect(screen.getByText("tokens card")).toBeInTheDocument();
    expect(screen.queryByText("identity providers card")).not.toBeInTheDocument();
  });

  it("shows a managed gateway's identity providers", () => {
    globalConfig.featureFlags = { ...globalConfig.featureFlags, enableSelfHostedGateways: false };
    renderGateway("MANAGED");

    expect(screen.getByText("identity providers card")).toBeInTheDocument();
    expect(screen.queryByText("setup steps")).not.toBeInTheDocument();
    expect(screen.queryByText("tokens card")).not.toBeInTheDocument();
  });
});

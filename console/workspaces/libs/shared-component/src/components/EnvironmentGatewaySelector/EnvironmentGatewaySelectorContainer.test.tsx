/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import type { Environment, GatewayResponse, GatewayType } from "@agent-management-platform/types";
import { EnvironmentGatewaySelector } from "./EnvironmentGatewaySelector";

// Covers the data-fetching wrapper; the view's selection logic lives in
// EnvironmentGatewaySelector.test.tsx.
const useListEnvironments = vi.fn();
const useListGateways = vi.fn();
vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: (...args: unknown[]) => useListEnvironments(...args),
  useListGateways: (...args: unknown[]) => useListGateways(...args),
}));

const env = (id: string, name: string): Environment => ({
  id,
  name,
  displayName: name,
  dataplaneRef: "dp-1",
  isProduction: false,
  createdAt: "2026-01-01T00:00:00Z",
});

const gateway = (uuid: string, envId: string, gatewayType: GatewayType): GatewayResponse =>
  ({
    uuid,
    organizationName: "org",
    name: uuid,
    displayName: `Gateway ${uuid}`,
    gatewayType,
    hosting: "MANAGED",
    vhost: "example.com",
    isCritical: false,
    status: "ACTIVE",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    environments: [{ id: envId, name: envId, displayName: envId }],
  }) as GatewayResponse;

const renderSelector = () =>
  render(
    <FeatureManagementProvider featureFlags={{}}>
      <ThemeProvider theme={createTheme()}>
        <EnvironmentGatewaySelector orgId="acme" value={[]} onChange={vi.fn()} />
      </ThemeProvider>
    </FeatureManagementProvider>,
  );

const NO_EGRESS = "No egress-capable gateway is attached to this environment.";

beforeEach(() => {
  useListEnvironments.mockReturnValue({
    data: [env("env-a", "Alpha"), env("env-b", "Beta")],
    isLoading: false,
  });
  useListGateways.mockReturnValue({ data: { gateways: [] }, isLoading: false });
});

describe("EnvironmentGatewaySelector", () => {
  it("lists the org's environments and gateways, asking for up to 500 gateways", () => {
    useListGateways.mockReturnValue({
      data: { gateways: [gateway("gw-1", "env-a", "EGRESS"), gateway("gw-2", "env-b", "BOTH")] },
      isLoading: false,
    });
    renderSelector();

    expect(useListEnvironments).toHaveBeenCalledWith({ orgName: "acme" });
    expect(useListGateways).toHaveBeenCalledWith({ orgName: "acme" }, { limit: 500 });
    expect(screen.getByRole("checkbox", { name: "Alpha" })).toBeEnabled();
    expect(screen.getByRole("checkbox", { name: "Beta" })).toBeEnabled();
  });

  it("does not offer ingress-only gateways as placement targets", () => {
    useListGateways.mockReturnValue({
      data: { gateways: [gateway("gw-1", "env-a", "INGRESS"), gateway("gw-2", "env-b", "EGRESS")] },
      isLoading: false,
    });
    renderSelector();

    expect(screen.getByRole("checkbox", { name: "Alpha" })).toBeDisabled();
    expect(screen.getAllByText(NO_EGRESS)).toHaveLength(1);
  });

  it.each([
    ["environments", () => useListEnvironments.mockReturnValue({ data: undefined, isLoading: true })],
    ["gateways", () => useListGateways.mockReturnValue({ data: undefined, isLoading: true })],
  ])("shows placeholders while %s load", (_label, setLoading) => {
    setLoading();
    const { container } = renderSelector();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(container.querySelectorAll(".MuiSkeleton-root")).toHaveLength(3);
  });
});

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

import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import {
  FeatureManagementProvider,
  type FeatureFlags,
} from "@agent-management-platform/feature-management";
import { GatewayIdentityProvidersCard } from "./GatewayIdentityProvidersCard";
import { identityProviderConfigMode } from "./ManageIdentityProviderDialog";

vi.mock("@agent-management-platform/api-client", () => ({
  useListIdentityProviders: () => ({ data: { list: [] }, isLoading: false, error: null }),
}));

vi.mock("./ManageIdentityProviderDialog", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./ManageIdentityProviderDialog")>()),
  ManageIdentityProviderDialog: () => null,
}));

const renderCard = (
  selfHosted: boolean,
  featureFlags: Partial<FeatureFlags> = {},
  canWrite = true,
) =>
  render(
    <FeatureManagementProvider
      featureFlags={featureFlags}
      featureAccess={{ gateway_write: canWrite }}
    >
      <GatewayIdentityProvidersCard
        orgId="acme"
        gatewayId="gw-1"
        gatewayName="Gateway 1"
        environments={[]}
        selfHosted={selfHosted}
      />
    </FeatureManagementProvider>,
  );

const addButton = () => screen.queryByRole("button", { name: "Add identity provider" });

describe("GatewayIdentityProvidersCard adding issuers", () => {
  it("is hidden on a managed gateway whose config cannot be patched", () => {
    renderCard(false);

    expect(addButton()).not.toBeInTheDocument();
    expect(
      screen.getByText(/Issuer changes can't be applied to it from here\./),
    ).toBeInTheDocument();
  });

  it("is offered on a managed gateway in managed mode", () => {
    renderCard(false, { enableIdentityProviderManagedMode: true });

    expect(addButton()).toBeInTheDocument();
  });

  it("is offered on a self-hosted gateway, whose owner edits its config", () => {
    renderCard(true);

    expect(addButton()).toBeInTheDocument();
  });

  it("tells a read-only viewer the truth about a gateway that accepts custom issuers", () => {
    renderCard(true, {}, false);

    expect(addButton()).not.toBeInTheDocument();
    expect(
      screen.getByText("No custom token issuers have been added to this gateway."),
    ).toBeInTheDocument();
  });
});

describe("identityProviderConfigMode", () => {
  it.each([
    [true, false, "manual"],
    [true, true, "manual"],
    [false, true, "applied"],
    [false, false, "fixed"],
  ] as const)("selfHosted=%s managedMode=%s is %s", (selfHosted, flag, want) => {
    expect(identityProviderConfigMode(selfHosted, flag)).toBe(want);
  });
});

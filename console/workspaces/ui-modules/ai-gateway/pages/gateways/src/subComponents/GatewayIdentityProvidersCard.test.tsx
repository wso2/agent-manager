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
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import { GatewayIdentityProvidersCard } from "./GatewayIdentityProvidersCard";
import {
  identityProviderConfigMode,
  type IdentityProviderConfigMode,
} from "./ManageIdentityProviderDialog";

vi.mock("@agent-management-platform/api-client", () => ({
  useListIdentityProviders: () => ({ data: { list: [] }, isLoading: false, error: null }),
}));

vi.mock("./ManageIdentityProviderDialog", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./ManageIdentityProviderDialog")>()),
  ManageIdentityProviderDialog: () => null,
}));

const renderCard = (configMode: IdentityProviderConfigMode, canWrite = true) =>
  render(
    <FeatureManagementProvider featureAccess={{ gateway_write: canWrite }}>
      <GatewayIdentityProvidersCard
        orgId="acme"
        gatewayId="gw-1"
        gatewayName="Gateway 1"
        environments={[]}
        configMode={configMode}
      />
    </FeatureManagementProvider>,
  );

const addButton = () => screen.queryByRole("button", { name: "Add identity provider" });

describe("GatewayIdentityProvidersCard adding issuers", () => {
  it("is hidden on a gateway nothing applies issuer changes to", () => {
    renderCard("fixed");

    expect(addButton()).not.toBeInTheDocument();
    expect(
      screen.getByText(/Issuer changes can't be applied to it from here\./),
    ).toBeInTheDocument();
  });

  it("is offered on a gateway whose config the service patches", () => {
    renderCard("applied");

    expect(addButton()).toBeInTheDocument();
  });

  it("is offered on a self-hosted gateway, whose owner edits its config", () => {
    renderCard("manual");

    expect(addButton()).toBeInTheDocument();
  });

  it("tells a read-only viewer the truth about a gateway that accepts custom issuers", () => {
    renderCard("manual", false);

    expect(addButton()).not.toBeInTheDocument();
    expect(
      screen.getByText("No custom token issuers have been added to this gateway."),
    ).toBeInTheDocument();
  });
});

describe("identityProviderConfigMode", () => {
  it.each([
    ["APPLIED", "MANAGED", "applied"],
    ["MANUAL", "SELF_HOSTED", "manual"],
    ["UNSUPPORTED", "MANAGED", "fixed"],
  ] as const)("a gateway reporting %s is %s", (identityProviderMode, hosting, want) => {
    expect(identityProviderConfigMode({ identityProviderMode, hosting })).toBe(want);
  });

  it("follows the service's report over the hosting", () => {
    // The service decides: a managed gateway is only applied where it says so.
    expect(identityProviderConfigMode({ identityProviderMode: "APPLIED", hosting: "MANAGED" }))
      .toBe("applied");
    expect(
      identityProviderConfigMode({ identityProviderMode: "UNSUPPORTED", hosting: "MANAGED" }),
    ).toBe("fixed");
  });

  it("assumes nothing is applied when the service does not report a mode", () => {
    expect(identityProviderConfigMode({ hosting: "SELF_HOSTED" })).toBe("manual");
    expect(identityProviderConfigMode({ hosting: "MANAGED" })).toBe("fixed");
    expect(identityProviderConfigMode({})).toBe("fixed");
  });
});

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
import {
  FeatureManagementProvider,
  type FeatureFlags,
} from "@agent-management-platform/feature-management";
import { GatewayIdentityProvidersCard } from "./GatewayIdentityProvidersCard";
import { identityProviderConfigMode } from "./ManageIdentityProviderDialog";

const hoisted = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
}));

// MUI renders are slow on a busy machine; leave headroom over the 5s default.
vi.setConfig({ testTimeout: 30_000 });

vi.mock("@agent-management-platform/api-client", () => ({
  useListIdentityProviders: () => hoisted.query,
}));

vi.mock("./ManageIdentityProviderDialog", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./ManageIdentityProviderDialog")>()),
  ManageIdentityProviderDialog: ({
    open,
    mode,
    provider,
    lockedGateway,
    onClose,
  }: {
    open: boolean;
    mode: string;
    provider?: { name: string } | null;
    lockedGateway?: { uuid: string; name: string };
    onClose: () => void;
  }) =>
    open ? (
      <div>
        dialog {mode} {provider?.name ?? "new"} on {lockedGateway?.name}
        <button onClick={onClose}>close dialog</button>
      </div>
    ) : null,
}));

const ENVS = [{ id: "e1", name: "dev", displayName: "Development" }];

const renderCard = ({
  canWrite = true,
  selfHosted = true,
  featureFlags = {},
}: {
  canWrite?: boolean;
  selfHosted?: boolean;
  featureFlags?: Partial<FeatureFlags>;
} = {}) =>
  render(
    <FeatureManagementProvider
      featureFlags={featureFlags}
      featureAccess={{ gateway_write: canWrite }}
    >
      <GatewayIdentityProvidersCard
        orgId="acme"
        gatewayId="gw-1"
        gatewayName="Edge"
        environments={ENVS as never}
        selfHosted={selfHosted}
      />
    </FeatureManagementProvider>,
  );

const providers = [
  { name: "AcmeIdP", type: "custom", issuer: "https://idp.acme", gatewayId: "gw-1", environmentName: "dev" },
  { name: "Platform", type: "system", issuer: "", gatewayId: "gw-1", environmentName: "dev" },
  { name: "OtherIdP", type: "custom", issuer: "https://other", gatewayId: "gw-2", environmentName: "dev" },
];

const addButton = () => screen.queryByRole("button", { name: "Add identity provider" });

beforeEach(() => {
  hoisted.query = { data: { list: providers }, isLoading: false, error: null };
});

describe("GatewayIdentityProvidersCard", () => {
  it("lists only this gateway's providers", () => {
    renderCard();
    expect(screen.getByText("AcmeIdP")).toBeInTheDocument();
    expect(screen.getByText("Platform")).toBeInTheDocument();
    expect(screen.queryByText("OtherIdP")).not.toBeInTheDocument();
    expect(screen.getByText("Custom")).toBeInTheDocument();
    expect(screen.getByText("System")).toBeInTheDocument();
    expect(screen.getByText("https://idp.acme")).toBeInTheDocument();
  });

  it("opens the add dialog locked to this gateway", () => {
    renderCard();
    fireEvent.click(screen.getByRole("button", { name: "Add identity provider" }));
    expect(screen.getByText("dialog upsert new on Edge")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "close dialog" }));
    expect(screen.queryByText(/dialog upsert/)).not.toBeInTheDocument();
  });

  it("offers removal of a custom provider on hover, but never a system one", () => {
    renderCard();
    const systemRow = screen.getByText("Platform").closest("tr")!;
    fireEvent.mouseEnter(systemRow);
    expect(screen.queryByRole("button", { name: "Remove identity provider" })).not.toBeInTheDocument();

    const customRow = screen.getByText("AcmeIdP").closest("tr")!;
    fireEvent.mouseEnter(customRow);
    fireEvent.click(screen.getByRole("button", { name: "Remove identity provider" }));
    expect(screen.getByText("dialog delete AcmeIdP on Edge")).toBeInTheDocument();
  });

  it("offers neither add nor remove without write access", () => {
    renderCard({ canWrite: false });
    fireEvent.mouseEnter(screen.getByText("AcmeIdP").closest("tr")!);
    expect(addButton()).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove identity provider" })).not.toBeInTheDocument();
  });

  it("offers adding from the empty state", () => {
    hoisted.query = { data: { list: [providers[2]] }, isLoading: false, error: null };
    renderCard();
    expect(screen.getByText("No identity providers configured")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add identity provider" }));
    expect(screen.getByText("dialog upsert new on Edge")).toBeInTheDocument();
  });

  it("shows the load error", () => {
    hoisted.query = { data: undefined, isLoading: false, error: new Error("forbidden") };
    renderCard();
    expect(screen.getByText("forbidden")).toBeInTheDocument();
    expect(addButton()).not.toBeInTheDocument();
  });

  it("shows no providers or empty state while loading", () => {
    hoisted.query = { data: undefined, isLoading: true, error: null };
    renderCard();
    expect(screen.queryByText("No identity providers configured")).not.toBeInTheDocument();
    expect(screen.queryByText("AcmeIdP")).not.toBeInTheDocument();
  });
});

describe("GatewayIdentityProvidersCard adding issuers", () => {
  beforeEach(() => {
    hoisted.query = { data: { list: [] }, isLoading: false, error: null };
  });

  it("is hidden on a managed gateway whose config cannot be patched", () => {
    renderCard({ selfHosted: false });

    expect(addButton()).not.toBeInTheDocument();
    expect(
      screen.getByText(/Issuer changes can't be applied to it from here\./),
    ).toBeInTheDocument();
  });

  it("is offered on a managed gateway in managed mode", () => {
    renderCard({ selfHosted: false, featureFlags: { enableIdentityProviderManagedMode: true } });

    expect(addButton()).toBeInTheDocument();
  });

  it("is offered on a self-hosted gateway, whose owner edits its config", () => {
    renderCard({ selfHosted: true });

    expect(addButton()).toBeInTheDocument();
  });

  it("tells a read-only viewer the truth about a gateway that accepts custom issuers", () => {
    renderCard({ selfHosted: true, canWrite: false });

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

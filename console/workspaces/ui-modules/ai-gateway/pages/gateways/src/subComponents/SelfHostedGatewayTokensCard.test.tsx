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
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import { SelfHostedGatewayTokensCard } from "./SelfHostedGatewayTokensCard";

const hoisted = vi.hoisted(() => ({
  query: {} as Record<string, unknown>,
  createToken: vi.fn(),
  revokeToken: vi.fn(),
  copy: vi.fn(),
  addConfirmation: vi.fn(),
}));

// MUI renders are slow on a busy machine; leave headroom over the 5s default.
vi.setConfig({ testTimeout: 30_000 });

vi.mock("@agent-management-platform/api-client", () => ({
  useSelfHostedGatewayTokens: () => hoisted.query,
  useCreateSelfHostedGatewayToken: () => ({ mutateAsync: hoisted.createToken, isPending: false }),
  useRevokeSelfHostedGatewayToken: () => ({ mutateAsync: hoisted.revokeToken }),
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  copyToClipboard: hoisted.copy,
  getErrorMessage: (e: unknown) => (e instanceof Error ? e.message : ""),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));

const CREATED = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString();
const token = (id: string, status: "active" | "revoked") => ({
  id,
  prefix: `${id}-pre`,
  status,
  createdAt: CREATED,
});
const masked = (id: string) => `${id}-pre-••••••••`;

const renderCard = (canManageTokens = true) =>
  render(
    <FeatureManagementProvider featureAccess={{ gateway_token_write: canManageTokens }}>
      <SelfHostedGatewayTokensCard orgName="acme" gatewayId="gw-1" />
    </FeatureManagementProvider>,
  );

const newTokenButton = () => screen.getByRole("button", { name: "New token" });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.query = {
    data: { list: [token("tok-a", "active"), token("tok-b", "revoked")] },
    isLoading: false,
    error: null,
  };
});

describe("SelfHostedGatewayTokensCard", () => {
  it("lists a token by its masked start, not by its record id", () => {
    renderCard();
    expect(screen.getByText(masked("tok-a"))).toBeInTheDocument();
    expect(screen.queryByText("tok-a")).not.toBeInTheDocument();
  });

  it("lists tokens with their status and age", () => {
    renderCard();
    expect(screen.getByText(masked("tok-a"))).toBeInTheDocument();
    expect(screen.getByText(masked("tok-b"))).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("Revoked")).toBeInTheDocument();
    expect(screen.getAllByText("Created 2 days ago")).toHaveLength(2);
    // Only an active token can be revoked.
    expect(screen.getAllByRole("button", { name: "Revoke token" })).toHaveLength(1);
  });

  it("shows a new token once and lets the user copy it", async () => {
    hoisted.createToken.mockResolvedValue({ token: "minted-secret" });
    renderCard();
    fireEvent.click(newTokenButton());

    expect(await screen.findByText(/Token created. Copy it now/)).toBeInTheDocument();
    expect(hoisted.createToken).toHaveBeenCalledWith({ orgName: "acme", gatewayId: "gw-1" });
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));
    expect(hoisted.copy).toHaveBeenCalledWith("minted-secret");
  });

  it("shows why a token could not be created", async () => {
    hoisted.createToken.mockRejectedValue(new Error("quota exceeded"));
    renderCard();
    fireEvent.click(newTokenButton());
    expect(await screen.findByText("quota exceeded")).toBeInTheDocument();
  });

  it("falls back to a generic message when the error has none", async () => {
    hoisted.createToken.mockRejectedValue("nope");
    renderCard();
    fireEvent.click(newTokenButton());
    expect(await screen.findByText("Failed to create a token.")).toBeInTheDocument();
  });

  it("blocks a third active token", () => {
    hoisted.query = {
      data: { list: [token("tok-a", "active"), token("tok-c", "active")] },
      isLoading: false,
      error: null,
    };
    renderCard();
    expect(newTokenButton()).toBeDisabled();
  });

  it("revokes a token after confirmation", async () => {
    renderCard();
    fireEvent.click(screen.getByRole("button", { name: "Revoke token" }));

    const confirmation = hoisted.addConfirmation.mock.calls[0][0];
    expect(confirmation.title).toBe("Revoke token");
    expect(hoisted.revokeToken).not.toHaveBeenCalled();
    await confirmation.onConfirm();
    expect(hoisted.revokeToken).toHaveBeenCalledWith({
      orgName: "acme",
      gatewayId: "gw-1",
      tokenId: "tok-a",
    });
  });

  it("is read-only without the token permission", () => {
    renderCard(false);
    expect(screen.getByText(masked("tok-a"))).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "New token" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Revoke token" })).not.toBeInTheDocument();
  });

  it("says when there are no tokens", () => {
    hoisted.query = { data: { list: [] }, isLoading: false, error: null };
    renderCard();
    expect(screen.getByText("No tokens yet.")).toBeInTheDocument();
  });

  it("reports a failed load", () => {
    hoisted.query = { data: undefined, isLoading: false, error: new Error("boom") };
    renderCard();
    expect(screen.getByText("Failed to load tokens.")).toBeInTheDocument();
    expect(screen.queryByText("No tokens yet.")).not.toBeInTheDocument();
  });

  it("shows neither tokens nor the empty message while loading", () => {
    hoisted.query = { data: undefined, isLoading: true, error: null };
    renderCard();
    expect(screen.queryByText("No tokens yet.")).not.toBeInTheDocument();
    expect(screen.queryByText("Failed to load tokens.")).not.toBeInTheDocument();
  });
});

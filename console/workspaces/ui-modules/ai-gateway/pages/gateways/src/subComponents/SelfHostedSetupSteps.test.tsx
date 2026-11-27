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
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import { SelfHostedSetupSteps } from "./SelfHostedSetupSteps";
import { forgetNewGatewayToken, keepNewGatewayToken } from "./newGatewayToken";

const hoisted = vi.hoisted(() => ({
  refetch: vi.fn(),
  createToken: vi.fn(),
  revokeToken: vi.fn(),
  copy: vi.fn(),
  gateway: { status: "INACTIVE" } as Record<string, unknown>,
}));

// MUI renders are slow on a busy machine; leave headroom over the 5s default.
vi.setConfig({ testTimeout: 30_000 });

vi.mock("@agent-management-platform/api-client", () => ({
  useSelfHostedGatewayTokens: () => ({ refetch: hoisted.refetch }),
  useCreateSelfHostedGatewayToken: () => ({ mutateAsync: hoisted.createToken }),
  useRevokeSelfHostedGatewayToken: () => ({ mutateAsync: hoisted.revokeToken }),
  useGetGateway: () => ({
    data: hoisted.gateway,
    error: null,
    isFetching: false,
    refetch: vi.fn(),
  }),
  isGatewayActive: (gateway?: { status?: string }) => gateway?.status === "ACTIVE",
}));

vi.mock("@agent-management-platform/shared-component", () => ({
  copyToClipboard: hoisted.copy,
}));

// The token the add page left for this gateway, as the steps read it from the store.
const renderSteps = (initialToken: string | null = null, canManageTokens = true) => {
  if (initialToken) keepNewGatewayToken("gw-1", initialToken);
  return render(
    <FeatureManagementProvider featureAccess={{ gateway_token_write: canManageTokens }}>
      <SelfHostedSetupSteps orgName="acme" gatewayId="gw-1" />
    </FeatureManagementProvider>,
  );
};

beforeEach(() => {
  vi.clearAllMocks();
  forgetNewGatewayToken("gw-1");
  hoisted.gateway = { status: "INACTIVE" };
  hoisted.copy.mockResolvedValue(true);
  hoisted.revokeToken.mockResolvedValue({});
});

describe("SelfHostedSetupSteps", () => {
  it("shows the setup commands for the default version and control plane", () => {
    renderSteps("tok-secret");
    expect(screen.getByText("1. Download the gateway")).toBeInTheDocument();
    expect(screen.getByText("4. Start the gateway")).toBeInTheDocument();
    expect(screen.getByText(/wso2apip-ai-gateway-2026\.09\.24\.zip/, { selector: "pre" }))
      .toBeInTheDocument();
    expect(screen.getByText("docker compose up")).toBeInTheDocument();
    expect(screen.getByText(/CONTROLPLANE_HOST=localhost:9243/)).toBeInTheDocument();
  });

  it("keeps the token off screen but copies it in the configure command", async () => {
    renderSteps("tok-secret");
    expect(screen.getByText(/Copy the command now/)).toBeInTheDocument();
    expect(screen.getByText(/CONTROLPLANE_TOKEN=<your-gateway-token>/)).toBeInTheDocument();
    expect(screen.queryByText(/tok-secret/)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Copy configure command" }));
    expect(hoisted.copy).toHaveBeenCalledWith(expect.stringContaining("CONTROLPLANE_TOKEN=tok-secret"));
    expect(await screen.findByText("Copied")).toBeInTheDocument();
  });

  it("does not claim a copy that failed", async () => {
    hoisted.copy.mockResolvedValue(false);
    renderSteps("tok-secret");
    fireEvent.click(screen.getByRole("button", { name: "Copy start command" }));
    await waitFor(() => expect(hoisted.copy).toHaveBeenCalledWith("docker compose up"));
    expect(screen.queryByText("Copied")).not.toBeInTheDocument();
  });

  it("revokes the active tokens and mints a new one on Reconfigure", async () => {
    hoisted.refetch.mockResolvedValue({
      data: {
        list: [
          { id: "old-1", status: "active" },
          { id: "old-2", status: "revoked" },
        ],
      },
    });
    hoisted.createToken.mockResolvedValue({ token: "fresh", tokenId: "new-1" });
    renderSteps();

    expect(screen.queryByRole("button", { name: "Copy configure command" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Reconfigure" }));

    expect(await screen.findByText(/New configuration generated/)).toBeInTheDocument();
    expect(hoisted.refetch).toHaveBeenCalledWith({ throwOnError: true });
    expect(hoisted.revokeToken).toHaveBeenCalledTimes(1);
    expect(hoisted.revokeToken).toHaveBeenCalledWith({
      orgName: "acme",
      gatewayId: "gw-1",
      tokenId: "old-1",
    });
    expect(hoisted.createToken).toHaveBeenCalledWith({ orgName: "acme", gatewayId: "gw-1" });

    fireEvent.click(screen.getByRole("button", { name: "Copy configure command" }));
    expect(hoisted.copy).toHaveBeenCalledWith(expect.stringContaining("CONTROLPLANE_TOKEN=fresh"));
  });

  it("revokes and mints nothing when the token list fails to load", async () => {
    hoisted.refetch.mockRejectedValue(new Error("list failed"));
    renderSteps();
    fireEvent.click(screen.getByRole("button", { name: "Reconfigure" }));

    expect(await screen.findByText("list failed")).toBeInTheDocument();
    expect(hoisted.revokeToken).not.toHaveBeenCalled();
    expect(hoisted.createToken).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Reconfigure" })).toBeEnabled();
  });

  it("explains that the token is shown once to users who cannot manage tokens", () => {
    renderSteps(null, false);
    expect(screen.queryByRole("button", { name: "Reconfigure" })).not.toBeInTheDocument();
    expect(screen.getByText(/shown only once, when it is generated/)).toBeInTheDocument();
  });
});

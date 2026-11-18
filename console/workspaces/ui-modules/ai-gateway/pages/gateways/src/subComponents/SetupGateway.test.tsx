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
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { SetupGateway, type SetupGatewayState } from "./SetupGateway";
import { keepNewGatewayToken, newGatewayToken } from "./newGatewayToken";

const hoisted = vi.hoisted(() => ({
  gateway: { uuid: "gw-1", displayName: "Edge One", hosting: "SELF_HOSTED", status: "INACTIVE" },
  error: null as Error | null,
  isLoading: false,
  /** The first read is paused (offline): not loading, no data, no error. */
  paused: false,
  refetch: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetGateway: () => ({
    data: hoisted.error || hoisted.isLoading || hoisted.paused ? undefined : hoisted.gateway,
    isLoading: hoisted.isLoading,
    error: hoisted.error,
    refetch: hoisted.refetch,
  }),
  isGatewayActive: (gw?: { status?: string }) => gw?.status === "ACTIVE",
}));

vi.mock("@agent-management-platform/views", async () => {
  const React = await import("react");
  return {
    PageLayout: ({ title, children }: { title: string; children: React.ReactNode }) =>
      React.createElement("div", null, React.createElement("h1", null, title), children),
    LoadErrorNotice: ({ error, onRetry }: { error?: Error; onRetry?: () => void }) =>
      React.createElement(
        "div",
        null,
        `load error: ${error?.message}`,
        React.createElement("button", { onClick: onRetry }, "Try again"),
      ),
  };
});

vi.mock("./SelfHostedSetupSteps", () => ({
  SelfHostedSetupSteps: ({ initialToken }: { initialToken: string | null }) => (
    <div>steps token={initialToken ?? "none"}</div>
  ),
}));

const renderSetup = (state: SetupGatewayState | null = null) =>
  render(
    <MemoryRouter initialEntries={[{ pathname: "/org/acme/gateways/setup/gw-1", state }]}>
      <Routes>
        <Route path="/org/:orgId/gateways/setup/:gatewayId" element={<SetupGateway />} />
        <Route path="/org/:orgId/gateways/view/:gatewayId" element={<div>gateway view</div>} />
      </Routes>
    </MemoryRouter>,
  );

afterEach(() => {
  hoisted.gateway = { ...hoisted.gateway, hosting: "SELF_HOSTED", status: "INACTIVE" };
  hoisted.error = null;
  hoisted.isLoading = false;
  hoisted.paused = false;
});

describe("SetupGateway", () => {
  it("passes the token left by the add page to the steps, and forgets it on leaving", () => {
    keepNewGatewayToken("gw-1", "secret");
    renderSetup();

    expect(screen.getByRole("heading", { name: "Set up Edge One" })).toBeInTheDocument();
    expect(screen.getByText("steps token=secret")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Finish later" }));
    expect(screen.getByText("gateway view")).toBeInTheDocument();
    expect(newGatewayToken("gw-1")).toBeNull();
  });

  it("has no token after a reload, and says why when minting failed", () => {
    renderSetup({ tokenFailed: true, tokenError: "quota reached" });

    expect(screen.getByText("steps token=none")).toBeInTheDocument();
    expect(
      screen.getByText(/its token could not be generated\. quota reached\./),
    ).toBeInTheDocument();
  });

  it("keeps Done disabled until the gateway connects", () => {
    renderSetup();
    expect(screen.getByRole("button", { name: "Done" })).toBeDisabled();
  });

  it("enables Done once the gateway is active", () => {
    hoisted.gateway = { ...hoisted.gateway, status: "ACTIVE" };
    renderSetup();

    expect(screen.queryByRole("button", { name: "Finish later" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.getByText("gateway view")).toBeInTheDocument();
  });

  it("forgets the token however the page is left", () => {
    keepNewGatewayToken("gw-1", "secret");
    const { unmount } = renderSetup();

    unmount();
    expect(newGatewayToken("gw-1")).toBeNull();
  });

  it("shows an error instead of the steps for a gateway that cannot be loaded", () => {
    hoisted.error = new Error("gateway not found");
    renderSetup();

    expect(screen.getByText("load error: gateway not found")).toBeInTheDocument();
    expect(screen.queryByText(/^steps/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Done" })).not.toBeInTheDocument();
  });

  it("retries a failed load on the same page, keeping the token", () => {
    keepNewGatewayToken("gw-1", "secret");
    hoisted.error = new Error("boom");
    renderSetup();

    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(hoisted.refetch).toHaveBeenCalled();
    // Still on the page, so the cleanup that forgets the token has not run.
    expect(newGatewayToken("gw-1")).toBe("secret");
  });

  it("holds the steps and actions back until the gateway has loaded", () => {
    hoisted.isLoading = true;
    renderSetup();

    expect(screen.queryByText(/^steps/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Done" })).not.toBeInTheDocument();
  });

  it("holds them back while the first read is paused, which is not loading", () => {
    hoisted.paused = true;
    renderSetup();

    expect(screen.queryByText(/^steps/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Done" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Finish later" })).not.toBeInTheDocument();
  });

  it("sends a managed gateway to its page", () => {
    hoisted.gateway = { ...hoisted.gateway, hosting: "MANAGED" };
    renderSetup();

    expect(screen.getByText("gateway view")).toBeInTheDocument();
  });
});

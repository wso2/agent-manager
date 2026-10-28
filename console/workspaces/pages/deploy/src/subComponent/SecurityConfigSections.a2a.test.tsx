/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { createRef } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { ConfigurationResponse } from "@agent-management-platform/types";

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useListEnvironmentIdentityProviders: vi.fn(() => ({ data: undefined })),
}));

import { useGetAgent } from "@agent-management-platform/api-client";
import { SecurityConfigSections, type SecurityConfigHandle } from "./SecurityConfigSections";

const mockAgent = (subType: string) =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: { agentType: { type: "agent-api", subType } },
  } as unknown as ReturnType<typeof useGetAgent>);

const mockAgentOfType = (type: string, subType: string) =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: { agentType: { type, subType } },
  } as unknown as ReturnType<typeof useGetAgent>);

const configurations = (overrides: Partial<ConfigurationResponse> = {}): ConfigurationResponse => ({
  projectName: "proj",
  agentName: "agent",
  environment: "dev",
  enableApiKeySecurity: true,
  corsConfig: {
    enabled: true, allowOrigin: ["*"], allowMethods: ["GET", "POST"],
    allowHeaders: ["Content-Type", "A2A-Version"], allowCredentials: false,
  },
  agentCardCorsConfig: { inherit: true, enabled: true, allowOrigin: ["*"], allowHeaders: ["Content-Type"] },
  resilienceTimeoutSeconds: 30,
  configurations: { env: [] },
  ...overrides,
});

const renderSections = (cfg: ConfigurationResponse) => {
  const ref = createRef<SecurityConfigHandle>();
  render(
    <SecurityConfigSections
      ref={ref}
      orgName="org"
      projName="proj"
      agentName="agent"
      environment="dev"
      open
      configurations={cfg}
    />,
  );
  return ref;
};

describe("SecurityConfigSections for A2A agents", () => {
  beforeEach(() => vi.clearAllMocks());

  it("hides the request timeout and does not send it", () => {
    mockAgent("a2a-agent");
    const ref = renderSections(configurations());
    expect(screen.queryByText("Request Timeout")).not.toBeInTheDocument();
    expect(ref.current?.buildBody()).not.toHaveProperty("resilienceTimeoutSeconds");
  });

  it("shows A2A-Version as always allowed and keeps it out of the editable payload", () => {
    mockAgent("a2a-agent");
    const ref = renderSections(configurations());
    expect(screen.getByText("A2A-Version")).toBeInTheDocument();
    expect(ref.current?.buildBody().corsConfig?.allowHeaders).toEqual(["Content-Type"]);
  });

  it("sends an inheriting card by default", () => {
    mockAgent("a2a-agent");
    const ref = renderSections(configurations());
    expect(screen.getByText("Agent Card CORS")).toBeInTheDocument();
    expect(ref.current?.buildBody().agentCardCorsConfig).toEqual({ inherit: true });
  });

  it("warns when API key security is off", () => {
    mockAgent("a2a-agent");
    renderSections(configurations({ enableApiKeySecurity: false }));
    expect(
      screen.getByText(/can be called by anyone who can reach the gateway/),
    ).toBeInTheDocument();
  });

  it("leaves non-A2A agents unchanged", () => {
    mockAgent("chat-api");
    const ref = renderSections(configurations({ enableApiKeySecurity: false }));
    expect(screen.getByText("Request Timeout")).toBeInTheDocument();
    expect(screen.queryByText("Agent Card CORS")).not.toBeInTheDocument();
    expect(screen.queryByText(/can be called by anyone/)).not.toBeInTheDocument();
    expect(ref.current?.buildBody()).not.toHaveProperty("agentCardCorsConfig");
    expect(ref.current?.buildBody()).toHaveProperty("resilienceTimeoutSeconds", 30);
  });

  it("serves a proxied external A2A agent's inbound auth and CORS", () => {
    mockAgentOfType("external-agent-api", "a2a-agent");
    const ref = renderSections(configurations());
    expect(screen.getByText("CORS Configuration")).toBeInTheDocument();
    expect(ref.current?.buildBody()).toMatchObject({ enableApiKeySecurity: true });
  });

  describe("forward token", () => {
    const oauthConfigurations = (forwardToken: boolean) =>
      configurations({
        enableApiKeySecurity: false,
        enableOAuthSecurity: true,
        oauthConfig: { issuers: ["idp"], forwardToken },
      });
    const forwardTokenSwitch = () =>
      screen.getByRole("switch", { name: /Forward token to upstream/ });

    it("keeps the switch off and disabled for a proxied external A2A agent", () => {
      mockAgentOfType("external-agent-api", "a2a-agent");
      renderSections(oauthConfigurations(true));
      expect(forwardTokenSwitch()).not.toBeChecked();
      expect(forwardTokenSwitch()).toBeDisabled();
      expect(screen.getByText(/never forwarded to an external agent/)).toBeInTheDocument();
    });

    it("never sends a forwarded token for a proxied external A2A agent", () => {
      mockAgentOfType("external-agent-api", "a2a-agent");
      const ref = renderSections(oauthConfigurations(true));
      expect(ref.current?.buildBody().oauthConfig?.forwardToken).toBe(false);
    });

    it("lets other agents choose whether to forward the token", async () => {
      mockAgent("a2a-agent");
      const ref = renderSections(oauthConfigurations(true));
      expect(forwardTokenSwitch()).toBeChecked();
      expect(forwardTokenSwitch()).toBeEnabled();
      expect(ref.current?.buildBody().oauthConfig?.forwardToken).toBe(true);

      fireEvent.click(forwardTokenSwitch());
      await waitFor(() =>
        expect(ref.current?.buildBody().oauthConfig?.forwardToken).toBe(false),
      );
    });
  });

  it("renders nothing for an external agent that is not A2A", () => {
    mockAgentOfType("external-agent-api", "custom");
    const ref = renderSections(configurations());
    expect(screen.queryByText("CORS Configuration")).not.toBeInTheDocument();
    expect(ref.current?.buildBody()).toEqual({});
  });
});

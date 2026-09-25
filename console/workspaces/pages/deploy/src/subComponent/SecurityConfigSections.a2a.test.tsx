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
import { render, screen } from "@testing-library/react";
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
});

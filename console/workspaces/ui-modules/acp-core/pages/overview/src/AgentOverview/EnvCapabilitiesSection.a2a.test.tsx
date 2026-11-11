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

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { DeploymentStatus } from "@agent-management-platform/shared-component";
import type { ConfigurationResponse } from "@agent-management-platform/types";

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useGetAgentConfigurations: vi.fn(),
  useGetAgentEndpoints: vi.fn(),
  useListEnvironmentIdentityProviders: vi.fn(() => ({ data: undefined })),
}));

import {
  useGetAgent,
  useGetAgentConfigurations,
  useGetAgentEndpoints,
} from "@agent-management-platform/api-client";
import { EnvCapabilitiesSection } from "./EnvCapabilitiesSection";

const mockAgent = (subType: string) =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: { agentType: { type: "agent-api", subType } },
  } as unknown as ReturnType<typeof useGetAgent>);

const mockConfigurations = (overrides: Partial<ConfigurationResponse> = {}) =>
  vi.mocked(useGetAgentConfigurations).mockReturnValue({
    data: {
      projectName: "proj",
      agentName: "agent",
      environment: "dev",
      enableApiKeySecurity: true,
      corsConfig: { enabled: false, allowOrigin: [] },
      ...overrides,
    },
    isLoading: false,
    isError: false,
  } as unknown as ReturnType<typeof useGetAgentConfigurations>);

const mockEndpoints = () =>
  vi.mocked(useGetAgentEndpoints).mockReturnValue({
    data: {
      dev: {
        url: "https://gw.example.com/org/proj/agent",
        visibility: "external",
      },
    },
    isLoading: false,
    isError: false,
  } as unknown as ReturnType<typeof useGetAgentEndpoints>);

const renderSection = () =>
  render(
    <MemoryRouter>
      <EnvCapabilitiesSection
        orgId="org"
        projectId="proj"
        agentId="agent"
        envId="dev"
        deploymentStatus={DeploymentStatus.ACTIVE}
      />
    </MemoryRouter>,
  );

describe("EnvCapabilitiesSection auth-off warning", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockEndpoints();
  });

  it("warns when an A2A agent's endpoint has no authentication", () => {
    mockAgent("a2a-agent");
    mockConfigurations({ enableApiKeySecurity: false });
    renderSection();
    expect(
      screen.getByText(/can be called by anyone who can reach the gateway/),
    ).toBeInTheDocument();
  });

  it("does not warn when the A2A agent has API key auth", () => {
    mockAgent("a2a-agent");
    mockConfigurations({ enableApiKeySecurity: true });
    renderSection();
    expect(screen.queryByText(/can be called by anyone/)).not.toBeInTheDocument();
  });

  it("does not warn for a non-A2A agent with no authentication", () => {
    mockAgent("chat-api");
    mockConfigurations({ enableApiKeySecurity: false });
    renderSection();
    expect(screen.queryByText(/can be called by anyone/)).not.toBeInTheDocument();
  });
});

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

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(() => ({
    data: { provisioning: { type: "external" }, agentType: { type: "external-agent-api", subType: "a2a-agent" } },
  })),
  useGetAgentConfigurations: vi.fn(),
  useListEnvironmentIdentityProviders: vi.fn(() => ({ data: undefined })),
  useUpdateAgentDeploySettings: vi.fn(() => ({ mutate: vi.fn(), isPending: false })),
}));

import { useGetAgentConfigurations } from "@agent-management-platform/api-client";
import { ProxySecurityDrawer } from "./ProxySecurityDrawer";

const renderDrawer = () =>
  render(
    <ProxySecurityDrawer open onClose={vi.fn()} orgName="org" projName="proj" agentName="agent" environment="dev" />,
  );

beforeEach(() => vi.clearAllMocks());

describe("ProxySecurityDrawer", () => {
  it("holds Apply until the environment's configuration arrives, so defaults never overwrite it", () => {
    vi.mocked(useGetAgentConfigurations).mockReturnValue({
      data: undefined,
    } as unknown as ReturnType<typeof useGetAgentConfigurations>);
    renderDrawer();
    expect(screen.getByRole("button", { name: "Apply" })).toBeDisabled();
  });

  it("enables Apply once the configuration is loaded", () => {
    vi.mocked(useGetAgentConfigurations).mockReturnValue({
      data: { enableApiKeySecurity: true, configurations: { env: [] } },
    } as unknown as ReturnType<typeof useGetAgentConfigurations>);
    renderDrawer();
    expect(screen.getByRole("button", { name: "Apply" })).toBeEnabled();
  });
});

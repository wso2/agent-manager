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

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { useAgentIdentityBinding } from "@agent-management-platform/api-client";
import type { AgentThunderStatus } from "@agent-management-platform/types";
import { EnvAgentRolesGroupsSection } from "./EnvAgentRolesGroupsSection";

vi.mock("@agent-management-platform/api-client", () => ({
  useAgentIdentityBinding: vi.fn(),
  useProvisionAgentIdentity: () => ({ mutate: vi.fn() }),
  useRetryAgentIdentityProvisioning: () => ({ mutate: vi.fn() }),
}));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureFlag: () => true,
  useResourceAccess: () => ({ canWrite: true }),
}));

vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...await importOriginal<typeof import("@agent-management-platform/shared-component")>(),
  useAgentRolesAndGroups: () => ({ roles: [], groups: [], isLoading: false }),
}));

const identityId = "test-agent-identity";
const section = () => (
  <MemoryRouter>
    <EnvAgentRolesGroupsSection orgId="org" projectId="proj" agentId="agent" envId="dev" />
  </MemoryRouter>
);

function mockBinding(status?: AgentThunderStatus) {
  vi.mocked(useAgentIdentityBinding).mockReturnValue({
    binding: status
      ? { status, agentId: identityId, provisioningType: "internal", environmentName: "dev" }
      : undefined,
    provisioned: status === "completed",
    isLoading: false,
    isError: false,
    error: null,
  });
}

describe("Agent ID card provisioning", () => {
  it.each(["pending", "in_progress"] as const)("replaces the %s loader with the provisioned ID", (status) => {
    mockBinding(status);
    const { rerender } = render(section());
    expect(screen.getByRole("status")).toHaveTextContent("Provisioning Agent ID");
    expect(screen.queryByText(identityId)).not.toBeInTheDocument();

    mockBinding("completed");
    rerender(section());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getByText(identityId)).toBeInTheDocument();
  });

  it("shows Retry when provisioning fails", () => {
    mockBinding("failed");
    render(section());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("offers creation when there is no identity binding", () => {
    mockBinding();
    render(section());
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create agent ID" })).toBeInTheDocument();
  });
});

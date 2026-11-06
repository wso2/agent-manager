/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({ status: "failed" }));
vi.mock("@agent-management-platform/api-client", () => ({
  useListEnvironments: () => ({ data: [] }),
  useListAgentIdentityAgents: () => ({}),
  useListAgentIdentityRoles: () => ({}),
  useListAgentIdentityGroups: () => ({}),
  useAddAgentIdentityRoleAssignees: () => ({}),
  useRemoveAgentIdentityRoleAssignees: () => ({}),
  useAddAgentIdentityGroupMembers: () => ({}),
  useRemoveAgentIdentityGroupMembers: () => ({}),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  useAgentIdentityCredentials: () => ({ binding: { status: state.status, provisioningType: "external" }, provisioned: false }),
  useThunderInstanceForEnv: () => ({}),
  useAgentRolesAndGroups: () => ({ roles: [], groups: [] }),
  usePipelineEnvironmentsState: () => ({ environments: [{ name: "development" }, { name: "production" }] }),
  useUnsavedChangesGuard: vi.fn(),
  getErrorMessage: () => "",
  monospaceInputSx: {},
}));
vi.mock("@agent-management-platform/views", () => ({
  PageLayout: ({ children }: React.PropsWithChildren) => <div>{children}</div>,
  TextInput: () => <input />,
}));
import { AgentIdComponent } from "./AgentId.Component";

function Location() {
  const { pathname, search } = useLocation();
  return <div data-testid="location">{pathname + search}</div>;
}
function renderPage() {
  return render(<MemoryRouter initialEntries={["/org/acme/project/support/agents/helper/agent-id?identityEnv=development"]}>
    <Routes>
      <Route path="/org/:orgId/project/:projectId/agents/:agentId/agent-id" element={<AgentIdComponent />} />
      <Route path="/org/:orgId/project/:projectId/agents/:agentId" element={<Location />} />
    </Routes>
  </MemoryRouter>);
}

describe("identity provisioning recovery", () => {
  it("links a failed identity to the same agent Overview, on the failed environment", () => {
    state.status = "failed";
    renderPage();
    const link = screen.getByRole("link", { name: "Go to Overview" });
    expect(link).toHaveAttribute("href", "/org/acme/project/support/agents/helper?env=development");
    fireEvent.click(link);
    expect(screen.getByTestId("location")).toHaveTextContent("/org/acme/project/support/agents/helper?env=development");
  });
  it("does not offer recovery while provisioning is in progress", () => {
    state.status = "pending";
    renderPage();
    expect(screen.queryByRole("link", { name: "Go to Overview" })).not.toBeInTheDocument();
    expect(screen.getByText("Provisioning in progress…")).toBeInTheDocument();
  });
});

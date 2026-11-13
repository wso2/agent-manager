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

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { AgentResponse } from "@agent-management-platform/types";

// The drawer's api-client import reaches for a configured app shell at import
// time, so stub the module boundary and observe the mutation directly.
vi.mock("@agent-management-platform/api-client", () => ({
  useUpdateAgentBuildParameters: vi.fn(),
  useListGitSecrets: vi.fn(() => ({ data: undefined, isLoading: false })),
}));

import { useUpdateAgentBuildParameters } from "@agent-management-platform/api-client";
import { ConfigureBuildDrawer } from "./ConfigureBuildDrawer";

const mutate = vi.fn();

const makeAgent = (overrides: Partial<AgentResponse> = {}): AgentResponse => ({
  name: "support-bot",
  displayName: "Support Bot",
  description: "",
  createdAt: "2026-01-01T00:00:00Z",
  projectName: "proj",
  provisioning: {
    type: "internal",
    repository: { url: "https://github.com/acme/agent", branch: "main", appPath: "/", secretRef: null },
  },
  agentType: { type: "agent-api", subType: "chat-api" },
  build: {
    type: "buildpack",
    buildpack: { language: "python", languageVersion: "3.11", runCommand: "python main.py" },
  },
  ...overrides,
});

// The unsaved-changes guard navigates, so the drawer needs a router around it.
const renderDrawer = (agent: AgentResponse) =>
  render(
    <MemoryRouter>
      <ConfigureBuildDrawer open onClose={vi.fn()} agent={agent} orgId="org" projectId="proj" />
    </MemoryRouter>,
  );

const save = async () => {
  fireEvent.click(screen.getByRole("button", { name: "Update build configuration" }));
  await waitFor(() => expect(mutate).toHaveBeenCalled());
  return mutate.mock.calls[0][0].body;
};

describe("ConfigureBuildDrawer health checks", () => {
  beforeEach(() => {
    mutate.mockClear();
    vi.mocked(useUpdateAgentBuildParameters).mockReturnValue({
      mutate,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateAgentBuildParameters>);
  });

  // Leaving healthChecks out keeps the saved ones, so a save that didn't touch them
  // can't change them.
  it("leaves health checks out of a save that didn't change them", async () => {
    renderDrawer(makeAgent());
    expect(await save()).not.toHaveProperty("healthChecks");
  });

  it("shows the saved health checks and still leaves them out when unchanged", async () => {
    renderDrawer(makeAgent({ healthChecks: { readiness: { type: "HTTP", path: "/health" } } }));
    expect(screen.getByLabelText("Readiness path")).toHaveValue("/health");
    expect(await save()).not.toHaveProperty("healthChecks");
  });

  it("sends all the health checks when one was changed", async () => {
    renderDrawer(makeAgent({ healthChecks: { readiness: { type: "HTTP", path: "/health" } } }));
    fireEvent.change(screen.getByLabelText("Readiness interval (s)"), { target: { value: "10" } });
    expect((await save()).healthChecks).toEqual({
      readiness: { type: "HTTP", path: "/health", periodSeconds: 10 },
    });
  });

  // Liveness without startup restarts an agent that is still starting, so the drawer warns.
  it("warns when liveness is on and startup is off", () => {
    renderDrawer(makeAgent({ healthChecks: { startup: { enabled: false }, liveness: { type: "TCP" } } }));
    expect(screen.getByText(/Liveness is on and startup is off/)).toBeInTheDocument();
  });

  it("does not warn when the startup check is on", () => {
    renderDrawer(makeAgent({ healthChecks: { liveness: { type: "TCP" } } }));
    expect(screen.queryByText(/Liveness is on and startup is off/)).not.toBeInTheDocument();
  });
});

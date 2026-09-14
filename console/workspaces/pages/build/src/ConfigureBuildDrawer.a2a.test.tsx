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
  name: "my-agent",
  displayName: "My Agent",
  description: "",
  createdAt: "2026-01-01T00:00:00Z",
  projectName: "proj",
  provisioning: {
    type: "internal",
    repository: {
      url: "https://github.com/acme/agent",
      branch: "main",
      appPath: "/",
    },
  },
  agentType: { type: "agent-api", subType: "chat-api" },
  build: {
    type: "buildpack",
    buildpack: { language: "python", languageVersion: "3.11", runCommand: "python main.py" },
  },
  ...overrides,
});

const renderDrawer = (agent: AgentResponse) =>
  render(
    <ConfigureBuildDrawer
      open
      onClose={vi.fn()}
      agent={agent}
      orgId="org"
      projectId="proj"
    />,
  );

const submittedBody = () => mutate.mock.calls[0][0].body;

describe("ConfigureBuildDrawer A2A interface", () => {
  beforeEach(() => {
    mutate.mockClear();
    vi.mocked(useUpdateAgentBuildParameters).mockReturnValue({
      mutate,
      isPending: false,
    } as unknown as ReturnType<typeof useUpdateAgentBuildParameters>);
  });

  it("preselects A2A for an agent already built as one", async () => {
    renderDrawer(
      makeAgent({
        agentType: { type: "agent-api", subType: "a2a-agent" },
        inputInterface: { type: "HTTP", port: 9099 },
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Update Build Configuration" }));

    await waitFor(() => expect(mutate).toHaveBeenCalled());
    expect(submittedBody().agentType.subType).toBe("a2a-agent");
    expect(submittedBody().inputInterface).toEqual({ type: "HTTP", port: 9099 });
  });

  // An A2A agent is provisioned without the REST api-configuration trait and is
  // published to the gateway as a kind: Agent, so the service refuses to move an
  // agent across that line. The drawer must not offer the crossing either.
  it("offers no A2A option to an agent built as a chat agent", () => {
    renderDrawer(makeAgent());

    expect(screen.getByText("Chat Agent")).toBeInTheDocument();
    expect(screen.getByText("Custom API Agent")).toBeInTheDocument();
    expect(screen.queryByText("A2A Agent")).not.toBeInTheDocument();
  });

  it("offers an A2A agent no way back to chat or custom API", () => {
    renderDrawer(
      makeAgent({
        agentType: { type: "agent-api", subType: "a2a-agent" },
        inputInterface: { type: "HTTP", port: 9099 },
      }),
    );

    expect(screen.getByText("A2A Agent")).toBeInTheDocument();
    expect(screen.queryByText("Chat Agent")).not.toBeInTheDocument();
    expect(screen.queryByText("Custom API Agent")).not.toBeInTheDocument();
  });

  // An agent created before subtypes were recorded carries none. It is not A2A,
  // so it keeps the two REST choices and is offered no promotion.
  it("treats an agent with no recorded subtype as a REST agent", () => {
    renderDrawer(makeAgent({ agentType: undefined }));

    expect(screen.getByText("Chat Agent")).toBeInTheDocument();
    expect(screen.getByText("Custom API Agent")).toBeInTheDocument();
    expect(screen.queryByText("A2A Agent")).not.toBeInTheDocument();
  });

  it("keeps a custom API agent's schema and base path when it stays custom", async () => {
    renderDrawer(
      makeAgent({
        agentType: { type: "agent-api", subType: "custom-api" },
        inputInterface: {
          type: "HTTP",
          port: 8080,
          basePath: "/api/v1",
          schema: { path: "/openapi.yaml" },
        },
      }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Update Build Configuration" }));

    await waitFor(() => expect(mutate).toHaveBeenCalled());
    expect(submittedBody().agentType.subType).toBe("custom-api");
    expect(submittedBody().inputInterface).toEqual({
      type: "HTTP",
      port: 8080,
      basePath: "/api/v1",
      schema: { path: "/openapi.yaml" },
    });
  });
});

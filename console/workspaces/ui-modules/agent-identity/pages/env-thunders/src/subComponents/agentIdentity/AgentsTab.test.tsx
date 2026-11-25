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

import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { apiState, renderAt, resetApiState } from "../../__test__/mocks";
import { AgentsTab } from "./AgentsTab";

vi.mock(
  "@agent-management-platform/api-client",
  async () => (await import("../../__test__/mocks")).apiClientMock,
);

// Oxygen UI pages are slow to render under a loaded CI runner.
vi.setConfig({ testTimeout: 60_000 });

const agent = (
  agentName: string,
  projectName: string,
  status: string,
  thunderAgentId?: string,
) => ({
  agentName,
  projectName,
  status,
  thunderAgentId,
});

const renderTab = () =>
  renderAt(
    <AgentsTab />,
    "/org/:orgId/thunder-instances/agents",
    "/org/acme/thunder-instances/agents?envName=dev",
  );

beforeEach(() => {
  resetApiState();
});

describe("AgentsTab", () => {
  it("shows each agent's resolved name, project, status and agent ID", () => {
    apiState.queries.useListAgentIdentityAgents = {
      data: {
        agents: [
          agent("support-bot", "shop", "completed", "tid-1"),
          agent("pricing", "shop", "pending"),
        ],
      },
    };
    renderTab();

    expect(screen.getByText("support-bot label")).toBeInTheDocument();
    expect(screen.getAllByText("shop project")).toHaveLength(2);
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.getByText("tid-1")).toBeInTheDocument();
    // An agent without a binding yet has no agent ID.
    expect(screen.getByText("-")).toBeInTheDocument();
    expect(apiState.calls.useListAgentIdentityAgents?.[0]).toEqual([
      { orgName: "acme", envName: "dev" },
    ]);
  });

  it("searches across names, projects, status and agent IDs", () => {
    apiState.queries.useListAgentIdentityAgents = {
      data: {
        agents: [
          agent("support-bot", "shop", "completed", "tid-1"),
          agent("pricing", "finance", "failed", "tid-2"),
        ],
      },
    };
    renderTab();
    const search = screen.getByPlaceholderText("Search agents");

    fireEvent.change(search, { target: { value: "FINANCE" } });
    expect(screen.getByText("pricing label")).toBeInTheDocument();
    expect(screen.queryByText("support-bot label")).not.toBeInTheDocument();

    fireEvent.change(search, { target: { value: "tid-1" } });
    expect(screen.getByText("support-bot label")).toBeInTheDocument();
    expect(screen.queryByText("pricing label")).not.toBeInTheDocument();

    fireEvent.change(search, { target: { value: "nothing-here" } });
    expect(screen.getByText("No agents found")).toBeInTheDocument();
  });

  it("opens an agent's identity page", () => {
    apiState.queries.useListAgentIdentityAgents = {
      data: { agents: [agent("support-bot", "shop", "completed", "tid-1")] },
    };
    renderTab();

    fireEvent.click(screen.getByText("support-bot label"));

    expect(
      screen.getByText("at /org/acme/thunder-instances/agents/shop/support-bot?envName=dev"),
    ).toBeInTheDocument();
  });

  it("shows an empty state when the environment has no agents", () => {
    apiState.queries.useListAgentIdentityAgents = { data: { agents: [] } };
    renderTab();

    expect(screen.getByText("No agents yet")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search agents")).not.toBeInTheDocument();
  });

  it("shows skeleton rows rather than the empty state while loading", () => {
    apiState.queries.useListAgentIdentityAgents = { isLoading: true };
    renderTab();

    expect(screen.queryByText("No agents yet")).not.toBeInTheDocument();
    expect(screen.getByText("Agent ID")).toBeInTheDocument();
  });

  it("reports a load failure", () => {
    apiState.queries.useListAgentIdentityAgents = { error: new Error("boom") };
    renderTab();

    expect(screen.getByText("Failed to load agents. Please try again.")).toBeInTheDocument();
  });
});

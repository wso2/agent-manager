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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

type Confirmation = { title: string; description: string; onConfirm: () => void };

const hoisted = vi.hoisted(() => ({
  access: { agent_create: true, agent_delete: true } as Record<string, boolean>,
  agents: { data: undefined as unknown, isLoading: false, error: null as unknown },
  deleteAgent: vi.fn(),
  addConfirmation: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  ConsoleAction: {},
  useTrack: () => ({ track: vi.fn() }),
  useListAgents: () => hoisted.agents,
  useDeleteAgent: () => ({ mutate: hoisted.deleteAgent }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: (feature: string) => hoisted.access[feature],
}));
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));

import { AgentListing } from "./AgentListing";
import { AgentList } from "./AgentList";

const Destination = () => {
  const location = useLocation();
  return <div>{`at ${location.pathname} ${JSON.stringify(location.state)}`}</div>;
};

const renderAt = (element: React.ReactNode) =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme/project/shop/agents"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/project/:projectId/agents" element={element} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

const agent = (name: string, displayName: string, extra: Record<string, unknown> = {}) => ({
  name,
  displayName,
  projectName: "shop",
  createdAt: "2026-01-01T00:00:00Z",
  provisioning: { type: "internal" },
  agentType: { type: "agent-api", subType: "chat-api" },
  ...extra,
});

const someAgents = () => ({
  data: {
    agents: [
      agent("support-bot", "Support Bot", { labels: { env: "prod" } }),
      agent("sales-bot", "Sales Bot", { provisioning: { type: "external" }, agentType: {} }),
    ],
  },
  isLoading: false,
  error: null,
});

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.access = { agent_create: true, agent_delete: true };
  hoisted.agents = someAgents();
});

describe("AgentListing", () => {
  it("lists the project's agents with their type", () => {
    renderAt(<AgentListing />);

    expect(screen.getByText("Agents")).toBeInTheDocument();
    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.getByText("Platform · Chat")).toBeInTheDocument();
    expect(screen.getByText("Sales Bot")).toBeInTheDocument();
    expect(screen.getByText("External")).toBeInTheDocument();
  });

  it("opens the create-agent page from the header's New agent", () => {
    renderAt(<AgentListing />);

    fireEvent.click(screen.getByRole("button", { name: "New agent" }));

    expect(screen.getByText(/^at \/org\/acme\/project\/shop\/newAgent/)).toBeInTheDocument();
  });

  it("hides New agent from users who cannot create agents", () => {
    hoisted.access.agent_create = false;
    renderAt(<AgentListing />);

    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "New agent" })).not.toBeInTheDocument();
  });

  it("offers New agent only in the empty state when there are no agents", () => {
    hoisted.agents = { data: { agents: [] }, isLoading: false, error: null };
    renderAt(<AgentListing />);

    expect(screen.getByText("No agents found")).toBeInTheDocument();
    expect(screen.getByText("Create a new agent to get started")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "New agent" })).toHaveLength(1);
  });
});

describe("AgentList", () => {
  it("opens an agent when its row is clicked", () => {
    renderAt(<AgentList />);

    fireEvent.click(screen.getByText("Sales Bot"));

    expect(
      screen.getByText(/^at \/org\/acme\/project\/shop\/agents\/sales-bot/),
    ).toBeInTheDocument();
  });

  it("filters by name and by key:value label", () => {
    renderAt(<AgentList />);
    const search = screen.getByPlaceholderText("Search agents");

    fireEvent.change(search, { target: { value: "sales" } });
    expect(screen.queryByText("Support Bot")).not.toBeInTheDocument();
    expect(screen.getByText("Sales Bot")).toBeInTheDocument();

    fireEvent.change(search, { target: { value: "env:prod" } });
    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.queryByText("Sales Bot")).not.toBeInTheDocument();
  });

  it("says when no agent matches the search", () => {
    renderAt(<AgentList />);

    fireEvent.change(screen.getByPlaceholderText("Search agents"), {
      target: { value: "nothing" },
    });

    expect(
      screen.getByText("No agents match your search criteria. Try adjusting your search."),
    ).toBeInTheDocument();
  });

  it("asks before deleting an agent and deletes it on confirm", () => {
    renderAt(<AgentList />);

    const row = screen.getByText("Support Bot").closest("tr") as HTMLElement;
    // The row reveals Delete on hover, so the button is in the DOM but CSS-hidden.
    fireEvent.click(within(row).getByRole("button", { name: "Delete", hidden: true }));

    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    const confirmation = hoisted.addConfirmation.mock.calls[0][0] as Confirmation;
    expect(confirmation.title).toBe("Delete Agent?");
    expect(confirmation.description).toContain('"Support Bot"');
    expect(hoisted.deleteAgent).not.toHaveBeenCalled();

    confirmation.onConfirm();

    expect(hoisted.deleteAgent).toHaveBeenCalledWith({
      orgName: "acme",
      projName: "shop",
      agentName: "support-bot",
    });
  });

  it("offers no Delete without the agent_delete permission", () => {
    hoisted.access.agent_delete = false;
    renderAt(<AgentList />);

    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete", hidden: true })).not.toBeInTheDocument();
  });

  it("shows New agent beside the search when asked to, carrying a way back", () => {
    renderAt(<AgentList showCreateAction />);

    fireEvent.click(screen.getByRole("button", { name: "New agent" }));

    expect(
      screen.getByText(
        'at /org/acme/project/shop/newAgent {"from":"/org/acme/project/shop/agents"}',
      ),
    ).toBeInTheDocument();
  });

  it("hides the agents while they load", () => {
    hoisted.agents = { data: undefined, isLoading: true, error: null };
    renderAt(<AgentList />);

    expect(screen.queryByText("Support Bot")).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search agents")).not.toBeInTheDocument();
  });

  it("shows only the error when the agents could not be loaded", () => {
    hoisted.agents = { data: undefined, isLoading: false, error: new Error("Agents unavailable") };
    renderAt(<AgentList />);

    expect(screen.getByText("Agents unavailable")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search agents")).not.toBeInTheDocument();
  });

  it("keeps showing loaded agents alongside a refetch error", () => {
    hoisted.agents = { ...someAgents(), error: new Error("Refresh failed") };
    renderAt(<AgentList />);

    expect(screen.getByText("Refresh failed")).toBeInTheDocument();
    expect(screen.getByText("Support Bot")).toBeInTheDocument();
  });

  it("pages through more than five agents", () => {
    hoisted.agents = {
      data: { agents: Array.from({ length: 7 }, (_, i) => agent(`bot-${i}`, `Bot ${i}`)) },
      isLoading: false,
      error: null,
    };
    renderAt(<AgentList />);

    expect(screen.getByText("Bot 4")).toBeInTheDocument();
    expect(screen.queryByText("Bot 5")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Go to next page" }));

    expect(screen.getByText("Bot 5")).toBeInTheDocument();
    expect(screen.queryByText("Bot 0")).not.toBeInTheDocument();
  });
});

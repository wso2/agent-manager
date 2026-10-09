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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useGetAgentCard: vi.fn(),
  useRefreshAgentCard: vi.fn(),
  useSetAgentCardSource: vi.fn(),
  useDeleteAgentCardSource: vi.fn(),
}));

import {
  useDeleteAgentCardSource,
  useGetAgent,
  useGetAgentCard,
  useRefreshAgentCard,
  useSetAgentCardSource,
} from "@agent-management-platform/api-client";
import { EnvAgentCardSection } from "./EnvAgentCardSection";

const refresh = vi.fn();
const setSource = vi.fn();

const mockAgent = (subType: string) =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: { agentType: { type: "agent-api", subType } },
  } as unknown as ReturnType<typeof useGetAgent>);

const mockCard = (value: Partial<ReturnType<typeof useGetAgentCard>>) =>
  vi.mocked(useGetAgentCard).mockReturnValue({
    data: undefined, isLoading: false, isError: false, error: null, ...value,
  } as unknown as ReturnType<typeof useGetAgentCard>);

const renderSection = (external = false) =>
  render(
    <MemoryRouter>
      <EnvAgentCardSection orgId="org" projectId="proj" agentId="agent" envId="dev" external={external} />
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(useRefreshAgentCard).mockReturnValue(
    { mutate: refresh, isPending: false } as unknown as
      ReturnType<typeof useRefreshAgentCard>);
  vi.mocked(useSetAgentCardSource).mockReturnValue(
    { mutate: setSource, isPending: false } as unknown as
      ReturnType<typeof useSetAgentCardSource>);
  vi.mocked(useDeleteAgentCardSource).mockReturnValue(
    { mutate: vi.fn(), isPending: false } as unknown as
      ReturnType<typeof useDeleteAgentCardSource>);
});

describe("EnvAgentCardSection", () => {
  it("renders nothing for a non-A2A agent", () => {
    mockAgent("chat-api");
    mockCard({});
    const { container } = renderSection();
    expect(container).toBeEmptyDOMElement();
  });

  it("shows the fetched card's summary and skills", () => {
    mockAgent("a2a-agent");
    mockCard({
      data: {
        source: "platform", status: "fetched", sourceUrl: "https://gw/a/.well-known/agent-card.json", lastError: "",
        fetchedAt: "2026-10-07T10:00:00Z",
        card: {
          name: "Trip Planner", description: "Plans trips", version: "1.2.0",
          supportedInterfaces: [{ url: "https://gw/a/rpc", protocolBinding: "JSONRPC" }],
          skills: [{ name: "plan", description: "Plan a trip", tags: ["travel"] }],
        },
      },
    });
    renderSection();
    expect(screen.getByText("Trip Planner")).toBeInTheDocument();
    expect(screen.queryByText("Fetched")).not.toBeInTheDocument();
    expect(screen.getByText(`Fetched ${new Date("2026-10-07T10:00:00Z").toLocaleString()}`)).toBeInTheDocument();
    expect(screen.getByText("plan")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://gw/a/rpc")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy URL" })).toBeInTheDocument();
  });

  it("shows the last error when the fetch failed", () => {
    mockAgent("a2a-agent");
    mockCard({ data: { source: "platform", status: "failed", sourceUrl: "", lastError: "HTTP 404" } });
    renderSection();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.getByText(/HTTP 404/)).toBeInTheDocument();
  });

  it("refetches on demand", () => {
    mockAgent("a2a-agent");
    mockCard({ data: { source: "platform", status: "fetched", sourceUrl: "", lastError: "" } });
    renderSection();
    fireEvent.click(screen.getByRole("button", { name: "Refetch" }));
    expect(refresh).toHaveBeenCalledWith({ orgName: "org", projName: "proj", agentName: "agent", envId: "dev" });
  });

  it("asks an external agent with no source for a card URL", () => {
    mockAgent("a2a-agent");
    mockCard({ isError: true, error: Object.assign(new Error("nf"), { status: 404 }) });
    renderSection(true);
    fireEvent.change(screen.getByPlaceholderText(/agent-card\.json/), {
      target: { value: "https://agent.example/.well-known/agent-card.json" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    expect(setSource).toHaveBeenCalledWith(
      {
        params: { orgName: "org", projName: "proj", agentName: "agent", envId: "dev" },
        body: { url: "https://agent.example/.well-known/agent-card.json" },
      },
      { onSuccess: undefined },
    );
  });

  it("offers a refetch when a platform agent has no card fetch queued", () => {
    mockAgent("a2a-agent");
    mockCard({ isError: true, error: Object.assign(new Error("nf"), { status: 404 }) });
    renderSection();
    expect(screen.getByText(/no agent card has been fetched/i)).toBeInTheDocument();
    expect(screen.queryByText(/register this agent/i)).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/agent-card\.json/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Refetch" }));
    expect(refresh).toHaveBeenCalledWith({ orgName: "org", projName: "proj", agentName: "agent", envId: "dev" });
  });

  it("offers no refetch to an external agent with no source", () => {
    mockAgent("a2a-agent");
    mockCard({ isError: true, error: Object.assign(new Error("nf"), { status: 404 }) });
    renderSection(true);
    expect(screen.getByText(/register this agent/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Refetch" })).not.toBeInTheDocument();
  });

  it("survives a malformed card and shows the registered URL instead of the form", () => {
    mockAgent("a2a-agent");
    mockCard({
      data: {
        source: "external", status: "fetched", sourceUrl: "https://agent.example/.well-known/agent-card.json", lastError: "",
        card: {
          name: "Odd", description: {}, version: 3,
          supportedInterfaces: [null, { url: "https://x/rpc", protocolBinding: {} }, "str"],
          skills: [null, { name: { x: 1 }, tags: "a" }, { name: "dup" }, { name: "dup", tags: ["t", 5] }],
        },
      },
    });
    renderSection(true);
    expect(screen.getByText("Odd")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://x/rpc")).toBeInTheDocument();
    expect(screen.getAllByText("dup")).toHaveLength(2);
    expect(screen.getByText("https://agent.example/.well-known/agent-card.json")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit URL" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove" })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/agent-card\.json/)).not.toBeInTheDocument();
  });

  it("does not offer the URL form while the card is still loading", () => {
    mockAgent("a2a-agent");
    mockCard({ isLoading: true });
    renderSection(true);
    expect(screen.queryByPlaceholderText(/agent-card\.json/)).not.toBeInTheDocument();
  });

  it("does not offer the URL form when the card could not be loaded", () => {
    mockAgent("a2a-agent");
    mockCard({ isError: true, error: Object.assign(new Error("boom"), { status: 500 }) });
    renderSection(true);
    expect(screen.getByText(/unable to load the agent card/i)).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/agent-card\.json/)).not.toBeInTheDocument();
  });

  it("drops the stale card once its source is gone", () => {
    mockAgent("a2a-agent");
    mockCard({
      isError: true,
      error: Object.assign(new Error("nf"), { status: 404 }),
      data: {
        source: "external", status: "fetched", sourceUrl: "https://old.example/card.json", lastError: "",
        card: { name: "Old Agent", supportedInterfaces: [], skills: [] },
      },
    });
    renderSection(true);
    expect(screen.queryByText("Old Agent")).not.toBeInTheDocument();
    expect(screen.queryByText("https://old.example/card.json")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove" })).not.toBeInTheDocument();
    expect(screen.getByText(/register this agent/i)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/agent-card\.json/)).toBeInTheDocument();
  });

  it("does not double the v of a version that already has one", () => {
    mockAgent("a2a-agent");
    mockCard({
      data: {
        source: "platform", status: "fetched", sourceUrl: "", lastError: "",
        card: { name: "Trip Planner", version: "v2.0", supportedInterfaces: [], skills: [] },
      },
    });
    renderSection();
    expect(screen.getByText("v2.0")).toBeInTheDocument();
  });

  it("keeps Save disabled for a URL with no host, and caps its length", () => {
    mockAgent("a2a-agent");
    mockCard({ isError: true, error: Object.assign(new Error("nf"), { status: 404 }) });
    renderSection(true);
    const input = screen.getByPlaceholderText(/agent-card\.json/);
    fireEvent.change(input, { target: { value: "https://" } });
    expect(screen.getByRole("button", { name: /save/i })).toBeDisabled();
    expect(input).toHaveAttribute("maxLength", "2048");
  });

  it("caps the inline URL editor's length", () => {
    mockAgent("a2a-agent");
    mockCard({ data: { source: "external", status: "fetched", sourceUrl: "https://a.example/c", lastError: "" } });
    renderSection(true);
    fireEvent.click(screen.getByRole("button", { name: "Edit URL" }));
    expect(screen.getByRole("textbox", { name: "Agent card URL" })).toHaveAttribute("maxLength", "2048");
  });
});


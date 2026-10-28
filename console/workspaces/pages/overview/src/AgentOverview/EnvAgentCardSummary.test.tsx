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
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgent: vi.fn(),
  useGetAgentCard: vi.fn(),
}));

import { useGetAgent, useGetAgentCard } from "@agent-management-platform/api-client";
import { EnvAgentCardSummary } from "./EnvAgentCardSummary";

const CONFIGURE_HREF = "/org/org/project/proj/agents/agent/configure?tab=a2a&env=dev";

const mockAgent = (subType: string) =>
  vi.mocked(useGetAgent).mockReturnValue({
    data: { agentType: { type: "agent-api", subType } },
  } as unknown as ReturnType<typeof useGetAgent>);

const mockCard = (value: Partial<ReturnType<typeof useGetAgentCard>>) =>
  vi.mocked(useGetAgentCard).mockReturnValue({
    data: undefined, isLoading: false, isError: false, error: null, ...value,
  } as unknown as ReturnType<typeof useGetAgentCard>);

// The hook reads a 404 (no card source) as null.
const notFound = { data: null };

const renderSummary = (external = false) =>
  render(
    <MemoryRouter>
      <EnvAgentCardSummary orgId="org" projectId="proj" agentId="agent" envId="dev" external={external} />
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
  mockAgent("a2a-agent");
});

describe("EnvAgentCardSummary", () => {
  it("renders nothing for a non-A2A agent", () => {
    mockAgent("chat-api");
    mockCard({});
    const { container } = renderSummary();
    expect(container).toBeEmptyDOMElement();
  });

  it("shows only the provider, interfaces and skill names of a fetched card", () => {
    mockCard({
      data: {
        source: "external", status: "fetched", sourceUrl: "https://a.example/card.json", lastError: "",
        card: {
          name: "Trip Planner",
          provider: { organization: "Acme", url: "https://acme.example" },
          supportedInterfaces: [{ url: "https://gw/a/rpc", protocolBinding: "JSONRPC" }],
          skills: [{ name: "plan", description: "Plan a trip", tags: ["travel"] }, { name: "book" }],
        },
      },
    });
    renderSummary(true);
    expect(screen.getByText("Acme")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Open provider website" })).toHaveAttribute("href", "https://acme.example");
    expect(screen.getByText("plan")).toBeInTheDocument();
    expect(screen.getByText("book")).toBeInTheDocument();
    expect(screen.getByText("JSONRPC")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://gw/a/rpc")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy URL" })).toBeInTheDocument();
    expect(screen.queryByText("Trip Planner")).not.toBeInTheDocument();
    expect(screen.queryByText("Plan a trip")).not.toBeInTheDocument();
    expect(screen.queryByText("travel")).not.toBeInTheDocument();
    expect(screen.queryByText("https://a.example/card.json")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /configure/i })).toHaveAttribute("href", CONFIGURE_HREF);
  });

  it("does not link a non-http provider URL", () => {
    mockCard({
      data: {
        source: "platform", status: "fetched", sourceUrl: "", lastError: "",
        card: { provider: { organization: "Acme", url: "javascript:alert(1)" }, skills: [] },
      },
    });
    renderSummary();
    expect(screen.getByText("Acme")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Open provider website" })).not.toBeInTheDocument();
  });

  it("points an external agent with no URL at Configure → A2A", () => {
    mockCard(notFound);
    renderSummary(true);
    expect(screen.getByRole("link", { name: /register agent card url/i })).toHaveAttribute("href", CONFIGURE_HREF);
  });

  it("does not ask a platform agent to register a URL", () => {
    mockCard(notFound);
    renderSummary();
    expect(screen.queryByText(/register agent card url/i)).not.toBeInTheDocument();
    expect(screen.getByText(/not available yet/i)).toBeInTheDocument();
  });

  it("says the card is not available while it has not been fetched", () => {
    mockCard({ data: { source: "external", status: "failed", sourceUrl: "https://a.example/c", lastError: "HTTP 404" } });
    renderSummary(true);
    expect(screen.getByText(/not available yet/i)).toBeInTheDocument();
    expect(screen.queryByText(/HTTP 404/)).not.toBeInTheDocument();
    expect(screen.queryByText(/register agent card url/i)).not.toBeInTheDocument();
  });
});

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

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/api-client", () => ({
  useGetAgentCard: vi.fn(),
  useGetA2AProxy: vi.fn(),
  useIsSavingA2AProxy: vi.fn(() => false),
  useSetA2AProxy: vi.fn(),
  useDeleteA2AProxy: vi.fn(),
  useRefreshA2AProxy: vi.fn(),
  useRefreshAgentCard: vi.fn(),
  useSetAgentCardSource: vi.fn(),
  useDeleteAgentCardSource: vi.fn(),
}));
vi.mock("@agent-management-platform/deploy", () => ({
  ProxySecurityDrawer: ({ open }: { open: boolean }) => (open ? <div>security drawer</div> : null),
}));

import {
  useDeleteA2AProxy,
  useDeleteAgentCardSource,
  useGetA2AProxy,
  useGetAgentCard,
  useIsSavingA2AProxy,
  useRefreshA2AProxy,
  useRefreshAgentCard,
  useSetA2AProxy,
  useSetAgentCardSource,
} from "@agent-management-platform/api-client";
import { A2AAgentCardPanel } from "./A2AAgentCardPanel";

const refresh = vi.fn();
const setSource = vi.fn();
const params = { orgName: "org", projName: "proj", agentName: "agent", envId: "dev" };

const mockCard = (value: Partial<ReturnType<typeof useGetAgentCard>>) =>
  vi.mocked(useGetAgentCard).mockReturnValue({
    data: undefined, isLoading: false, isError: false, error: null, ...value,
  } as unknown as ReturnType<typeof useGetAgentCard>);

// The hooks read a 404 (no card source, or a direct environment) as null.
const noSource = { data: null };
const direct = { data: null, isSuccess: true };

const mockProxy = (value: Partial<ReturnType<typeof useGetA2AProxy>>) =>
  vi.mocked(useGetA2AProxy).mockReturnValue({
    data: undefined, isSuccess: false, isError: false, error: null, ...value,
  } as unknown as ReturnType<typeof useGetA2AProxy>);

const idleMutation = (hook: unknown) =>
  vi.mocked(hook as () => unknown).mockReturnValue(
    { mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false });

const renderPanel = (external = false) =>
  render(
    <MemoryRouter>
      <A2AAgentCardPanel orgId="org" projectId="proj" agentId="agent" envId="dev" external={external} />
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(useIsSavingA2AProxy).mockReturnValue(false);
  vi.mocked(useRefreshAgentCard).mockReturnValue(
    { mutate: refresh, isPending: false } as unknown as
      ReturnType<typeof useRefreshAgentCard>);
  vi.mocked(useSetAgentCardSource).mockReturnValue(
    { mutateAsync: setSource, isPending: false } as unknown as
      ReturnType<typeof useSetAgentCardSource>);
  vi.mocked(useDeleteAgentCardSource).mockReturnValue(
    { mutate: vi.fn(), isPending: false } as unknown as
      ReturnType<typeof useDeleteAgentCardSource>);
  setSource.mockResolvedValue(undefined);
  mockProxy(direct);
  idleMutation(useSetA2AProxy);
  idleMutation(useDeleteA2AProxy);
  idleMutation(useRefreshA2AProxy);
});

describe("A2AAgentCardPanel", () => {
  it("shows the fetched card's summary and skills", () => {
    mockCard({
      data: {
        source: "platform", status: "fetched", sourceUrl: "https://gw/a/.well-known/agent-card.json", lastError: "",
        fetchedAt: "2026-10-07T10:00:00Z",
        card: {
          name: "Trip Planner", description: "Plans trips", version: "1.2.0",
          supportedInterfaces: [
            { url: "https://gw/a/rpc", protocolBinding: "JSONRPC" },
            { protocolBinding: "HTTP+JSON" },
          ],
          skills: [{
            name: "plan", description: "Plan a trip", tags: ["travel"],
            inputModes: ["text/plain"], examples: ["Plan Paris"],
          }],
        },
      },
    });
    renderPanel();
    expect(screen.getByText("Trip Planner")).toBeInTheDocument();
    expect(screen.queryByText("Fetched")).not.toBeInTheDocument();
    expect(screen.getByText(`Fetched ${new Date("2026-10-07T10:00:00Z").toLocaleString()}`)).toBeInTheDocument();
    expect(screen.getByText("plan")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://gw/a/rpc")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy URL" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Copy URL" })).toHaveLength(1);
    expect(screen.queryByText("HTTP+JSON")).not.toBeInTheDocument();
    expect(screen.getByText("text/plain")).toBeInTheDocument();
    expect(screen.getByText("e.g. Plan Paris")).toBeInTheDocument();
    expect(screen.getByText("v1.2.0")).toBeInTheDocument();
  });

  it("shows the last error when the fetch failed", () => {
    mockCard({ data: { source: "platform", status: "failed", sourceUrl: "", lastError: "HTTP 404" } });
    renderPanel();
    expect(screen.getByText("Failed")).toBeInTheDocument();
    expect(screen.getByText(/HTTP 404/)).toBeInTheDocument();
  });

  it("refetches on demand", () => {
    mockCard({ data: { source: "platform", status: "fetched", sourceUrl: "", lastError: "" } });
    renderPanel();
    fireEvent.click(screen.getByRole("button", { name: "Refetch" }));
    expect(refresh).toHaveBeenCalledWith({ orgName: "org", projName: "proj", agentName: "agent", envId: "dev" });
  });

  it("asks an external agent with no source for a card URL", () => {
    mockCard(noSource);
    renderPanel(true);
    fireEvent.change(screen.getByPlaceholderText(/agent-card\.json/), {
      target: { value: "https://agent.example/.well-known/agent-card.json" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    expect(setSource).toHaveBeenCalledWith({
      params, body: { url: "https://agent.example/.well-known/agent-card.json" }, routingFollows: false,
    });
  });

  const fillNewUrlAndRoute = () => {
    mockCard(noSource);
    renderPanel(true);
    fireEvent.change(screen.getByPlaceholderText(/agent-card\.json/), {
      target: { value: "https://agent.example/.well-known/agent-card.json" },
    });
    fireEvent.click(screen.getByLabelText("Route through gateway"));
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
  };

  it("saves a new URL, then routes it through the gateway", async () => {
    const setProxy = vi.fn().mockResolvedValue(undefined);
    vi.mocked(useSetA2AProxy).mockReturnValue(
      { mutateAsync: setProxy, isPending: false } as unknown as ReturnType<typeof useSetA2AProxy>);
    fillNewUrlAndRoute();
    expect(setSource).toHaveBeenCalledWith(expect.objectContaining({ routingFollows: true }));
    await waitFor(() => expect(setProxy).toHaveBeenCalledWith({ params, body: {} }));
  });

  it("keeps the URL form disabled while the routing save that follows it is in flight", () => {
    vi.mocked(useSetA2AProxy).mockReturnValue(
      { mutateAsync: vi.fn(), isPending: true } as unknown as ReturnType<typeof useSetA2AProxy>);
    mockCard(noSource);
    renderPanel(true);
    fireEvent.change(screen.getByPlaceholderText(/agent-card\.json/), {
      target: { value: "https://agent.example/.well-known/agent-card.json" },
    });
    expect(screen.getByRole("button", { name: /save/i })).toBeDisabled();
    expect(screen.getByLabelText("Route through gateway")).toBeDisabled();
  });

  it("does not route a URL that failed to save", async () => {
    const setProxy = vi.fn().mockResolvedValue(undefined);
    vi.mocked(useSetA2AProxy).mockReturnValue(
      { mutateAsync: setProxy, isPending: false } as unknown as ReturnType<typeof useSetA2AProxy>);
    setSource.mockRejectedValue(new Error("boom"));
    fillNewUrlAndRoute();
    await waitFor(() => expect(setSource).toHaveBeenCalledTimes(1));
    await Promise.resolve();
    expect(setProxy).not.toHaveBeenCalled();
  });

  it("does not call the gateway API when routing is left off", async () => {
    const setProxy = vi.fn().mockResolvedValue(undefined);
    vi.mocked(useSetA2AProxy).mockReturnValue(
      { mutateAsync: setProxy, isPending: false } as unknown as ReturnType<typeof useSetA2AProxy>);
    mockCard(noSource);
    renderPanel(true);
    fireEvent.change(screen.getByPlaceholderText(/agent-card\.json/), {
      target: { value: "https://agent.example/.well-known/agent-card.json" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() => expect(setSource).toHaveBeenCalledTimes(1));
    await Promise.resolve();
    expect(setProxy).not.toHaveBeenCalled();
  });

  it("stops polling a parked card once the gateway publish failed, and says why", () => {
    mockCard({ data: { source: "external", status: "pending", sourceUrl: "https://agent.example/.well-known/agent-card.json", lastError: "" } });
    mockProxy({
      isSuccess: true,
      data: { status: "failed", lastError: "the agent cannot be proxied through the gateway", gatewayUrl: "", upstreamAuthHeader: "" },
    });
    renderPanel(true);
    expect(screen.getByText("Gateway publish failed")).toBeInTheDocument();
    expect(screen.queryByText("Pending")).not.toBeInTheDocument();
    expect(vi.mocked(useGetAgentCard))
      .toHaveBeenLastCalledWith(params, { poll: false });
  });

  const registeredCard = {
    source: "external", status: "fetched", sourceUrl: "https://agent.example/.well-known/agent-card.json", lastError: "",
  } as const;

  const spyOnProxyWrites = () => {
    const write = vi.fn().mockResolvedValue(undefined);
    for (const hook of [useSetA2AProxy, useDeleteA2AProxy]) {
      vi.mocked(hook as () => unknown).mockReturnValue(
        { mutate: write, mutateAsync: write, isPending: false });
    }
    return write;
  };

  // An unknown routing would read as direct, and saving it would drop a stored upstream secret.
  it("offers no routing controls when the gateway routing could not be loaded", () => {
    const write = spyOnProxyWrites();
    mockCard({ data: registeredCard });
    mockProxy({ isError: true, error: Object.assign(new Error("boom"), { status: 500 }) });
    renderPanel(true);
    expect(screen.getByText("Couldn't load gateway routing.")).toBeInTheDocument();
    expect(screen.queryByLabelText("Route through gateway")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
    expect(write).not.toHaveBeenCalled();
  });

  it("keeps the routing section mounted when a background refetch fails", () => {
    mockCard({ data: registeredCard });
    mockProxy({
      data: { status: "published", lastError: "", gatewayUrl: "https://gw.example/agent", upstreamAuthHeader: "" },
      isSuccess: false,
      isError: true,
      error: { status: 500 },
    });
    renderPanel(true);
    expect(screen.getByLabelText("Route through gateway")).toBeInTheDocument();
    expect(screen.queryByText("Couldn't load gateway routing.")).not.toBeInTheDocument();
  });

  it("keeps the routing section mounted but disabled while a routing save is in flight", () => {
    mockCard({ data: registeredCard });
    mockProxy(direct);
    vi.mocked(useIsSavingA2AProxy).mockReturnValue(true);
    renderPanel(true);
    expect(screen.getByLabelText("Route through gateway")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(screen.queryByText("Loading gateway routing…")).not.toBeInTheDocument();
    expect(screen.queryByText("Couldn't load gateway routing.")).not.toBeInTheDocument();
  });

  it("offers no routing controls while the gateway routing loads", () => {
    mockCard({ data: registeredCard });
    mockProxy({ isLoading: true });
    renderPanel(true);
    expect(screen.getByText("Loading gateway routing…")).toBeInTheDocument();
    expect(screen.queryByLabelText("Route through gateway")).not.toBeInTheDocument();
  });

  it("registers a URL without routing when the gateway routing could not be loaded", async () => {
    const write = spyOnProxyWrites();
    mockCard(noSource);
    mockProxy({ isError: true, error: Object.assign(new Error("boom"), { status: 500 }) });
    renderPanel(true);
    expect(screen.queryByLabelText("Route through gateway")).not.toBeInTheDocument();
    expect(screen.getByText("Couldn't load gateway routing.")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText(/agent-card\.json/), {
      target: { value: "https://agent.example/.well-known/agent-card.json" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() => expect(setSource).toHaveBeenCalledTimes(1));
    await Promise.resolve();
    expect(write).not.toHaveBeenCalled();
  });

  it("offers a refetch when a platform agent has no card fetch queued", () => {
    mockCard(noSource);
    renderPanel();
    expect(screen.getByText(/no agent card has been fetched/i)).toBeInTheDocument();
    expect(screen.queryByText(/register this agent/i)).not.toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/agent-card\.json/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Refetch" }));
    expect(refresh).toHaveBeenCalledWith({ orgName: "org", projName: "proj", agentName: "agent", envId: "dev" });
  });

  it("offers no refetch to an external agent with no source", () => {
    mockCard(noSource);
    renderPanel(true);
    expect(screen.getByText(/register this agent/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Refetch" })).not.toBeInTheDocument();
  });

  it("survives a malformed card and shows the registered URL instead of the form", () => {
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
    renderPanel(true);
    expect(screen.getByText("Odd")).toBeInTheDocument();
    expect(screen.getByDisplayValue("https://x/rpc")).toBeInTheDocument();
    expect(screen.getAllByText("dup")).toHaveLength(2);
    expect(screen.getByText("https://agent.example/.well-known/agent-card.json")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit URL" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Remove" })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/agent-card\.json/)).not.toBeInTheDocument();
  });

  it("does not offer the URL form while the card is still loading", () => {
    mockCard({ isLoading: true });
    renderPanel(true);
    expect(screen.queryByPlaceholderText(/agent-card\.json/)).not.toBeInTheDocument();
  });

  it("does not offer the URL form when the card could not be loaded", () => {
    mockCard({ isError: true, error: Object.assign(new Error("boom"), { status: 500 }) });
    renderPanel(true);
    expect(screen.getByText(/unable to load the agent card/i)).toBeInTheDocument();
    expect(screen.queryByPlaceholderText(/agent-card\.json/)).not.toBeInTheDocument();
  });

  it("does not double the v of a version that already has one", () => {
    mockCard({
      data: {
        source: "platform", status: "fetched", sourceUrl: "", lastError: "",
        card: { name: "Trip Planner", version: "v2.0", supportedInterfaces: [], skills: [] },
      },
    });
    renderPanel();
    expect(screen.getByText("v2.0")).toBeInTheDocument();
  });

  it("normalises a v-prefixed version", () => {
    mockCard({
      data: {
        source: "platform", status: "fetched", sourceUrl: "", lastError: "",
        card: { name: "Trip Planner", version: "v2.0", supportedInterfaces: [], skills: [] },
      },
    });
    renderPanel();
    expect(screen.getAllByText("v2.0")).toHaveLength(1);
  });

  it("keeps Save disabled for a URL with no host, and caps its length", () => {
    mockCard(noSource);
    renderPanel(true);
    const input = screen.getByPlaceholderText(/agent-card\.json/);
    fireEvent.change(input, { target: { value: "https://" } });
    expect(screen.getByRole("button", { name: /save/i })).toBeDisabled();
    expect(input).toHaveAttribute("maxLength", "2048");
  });

  it("caps the inline URL editor's length", () => {
    mockCard({ data: { source: "external", status: "fetched", sourceUrl: "https://a.example/c", lastError: "" } });
    renderPanel(true);
    fireEvent.click(screen.getByRole("button", { name: "Edit URL" }));
    expect(screen.getByRole("textbox", { name: "Agent card URL" })).toHaveAttribute("maxLength", "2048");
  });

  const editStoredSecretUrl = () => {
    mockCard({ data: registeredCard });
    mockProxy({ isSuccess: true, data: { status: "published", lastError: "", gatewayUrl: "", upstreamAuthHeader: "X-Key" } });
    renderPanel(true);
    fireEvent.click(screen.getByRole("button", { name: "Edit URL" }));
    return screen.getByRole("textbox", { name: "Agent card URL" });
  };

  it("keeps a card with a stored header value on its origin", () => {
    const input = editStoredSecretUrl();
    expect(screen.getByText(/clear the stored value first/)).toBeInTheDocument();
    fireEvent.change(input, { target: { value: "https://other.example/.well-known/agent-card.json" } });
    expect(screen.getByRole("button", { name: "Save agent card URL" })).toBeDisabled();
    fireEvent.change(input, { target: { value: "https://agent.example:443/v2/agent-card.json" } });
    expect(screen.getByRole("button", { name: "Save agent card URL" })).toBeEnabled();
  });

  it("lets a card with no stored header value move to another origin", () => {
    mockCard({ data: registeredCard });
    mockProxy({ isSuccess: true, data: { status: "published", lastError: "", gatewayUrl: "", upstreamAuthHeader: "" } });
    renderPanel(true);
    fireEvent.click(screen.getByRole("button", { name: "Edit URL" }));
    expect(screen.queryByText(/clear the stored value first/)).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole("textbox", { name: "Agent card URL" }), {
      target: { value: "https://other.example/.well-known/agent-card.json" },
    });
    expect(screen.getByRole("button", { name: "Save agent card URL" })).toBeEnabled();
  });

  it("refuses to send a header value to an http card URL", () => {
    mockCard(noSource);
    renderPanel(true);
    fireEvent.change(screen.getByPlaceholderText(/agent-card\.json/), {
      target: { value: "http://agent.example/.well-known/agent-card.json" },
    });
    fireEvent.click(screen.getByLabelText("Route through gateway"));
    fireEvent.change(screen.getByLabelText("Upstream header"), { target: { value: "X-Key" } });
    fireEvent.change(screen.getByLabelText("Header value"), { target: { value: "secret" } });
    expect(screen.getByText("The card URL must use https to send a header value")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save/i })).toBeDisabled();
  });
});

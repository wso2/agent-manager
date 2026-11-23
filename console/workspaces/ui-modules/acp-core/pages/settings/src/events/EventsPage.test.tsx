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
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { destination, subscription } from "./__test__/fixtures";

const hoisted = vi.hoisted(() => ({
  config: { data: { eventsEnabled: true } as unknown, isLoading: false, isError: false },
  tokenScopes: { enforced: false, resolved: true, scopes: new Set<string>() },
  subscriptions: { data: undefined as unknown, isLoading: false, error: undefined },
  destinations: { data: undefined as unknown },
  tabProps: {} as Record<string, Record<string, unknown>>,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useRuntimeConfig: () => hoisted.config,
  useTokenScopes: () => hoisted.tokenScopes,
  useListEventSubscriptions: () => hoisted.subscriptions,
  useListEventDestinations: () => hoisted.destinations,
}));

// The tabs and editors have tests of their own; here they only report what
// the page hands them and expose their callbacks.
vi.mock("./SubscriptionsTab", () => ({
  SubscriptionsTab: (p: Record<string, unknown>) => {
    hoisted.tabProps.subscriptions = p;
    return (
      <div>
        subscriptions tab
        <button onClick={p.onNew as () => void}>new</button>
        <button onClick={() => (p.onEdit as (s: unknown) => void)({ id: "s1" })}>edit</button>
        <button onClick={() => (p.onViewDeliveries as (id: string) => void)("s1")}>deliveries</button>
      </div>
    );
  },
}));
vi.mock("./DestinationsTab", () => ({
  DestinationsTab: (p: Record<string, unknown>) => {
    hoisted.tabProps.destinations = p;
    return (
      <div>
        destinations tab
        <button onClick={p.onAdd as () => void}>add</button>
        <button onClick={() => (p.onEdit as (d: unknown) => void)({ id: "d1" })}>edit</button>
        <button onClick={() => (p.onViewDeliveries as (id: string) => void)("d1")}>deliveries</button>
      </div>
    );
  },
}));
vi.mock("./DeliveryLogTab", () => ({
  DeliveryLogTab: (p: Record<string, unknown>) => {
    hoisted.tabProps.deliveries = p;
    return (
      <div>
        delivery log tab
        <button onClick={() => (p.onDestinationChange as (id?: string) => void)("d2")}>
          pick destination
        </button>
        <button onClick={p.onClearSubscription as () => void}>clear subscription</button>
      </div>
    );
  },
}));
vi.mock("./SubscriptionEditor", () => ({
  SubscriptionEditor: ({ listPath, levelName }: { listPath: string; levelName: string }) => (
    <div>{`subscription editor for ${levelName}, back to ${listPath}`}</div>
  ),
}));
vi.mock("./DestinationEditor", () => ({
  DestinationEditor: ({ listPath }: { listPath: string }) => (
    <div>{`destination editor, back to ${listPath}`}</div>
  ),
}));

import { AgentEvents, OrgEvents, ProjectEvents } from "./EventsPage";

const Where = () => {
  const location = useLocation();
  return (
    <div data-testid="where">
      {`${location.pathname}${location.search} ${JSON.stringify(location.state)}`}
    </div>
  );
};

const renderAt = (entry: string) =>
  render(
    // No v7_relativeSplatPath: the console's BrowserRouter doesn't opt in,
    // and the page resolves its list path with useResolvedPath(".").
    <MemoryRouter initialEntries={[entry]} future={{ v7_startTransition: true }}>
      <Routes>
        <Route path="/org/:orgId/events/*" element={<><OrgEvents /><Where /></>} />
        <Route path="/org/:orgId/project/:projectId/events/*" element={<><ProjectEvents /><Where /></>} />
        <Route
          path="/org/:orgId/project/:projectId/agents/:agentId/events/*"
          element={<><AgentEvents /><Where /></>}
        />
      </Routes>
    </MemoryRouter>,
  );

const where = () => screen.getByTestId("where").textContent;

// Full MUI pages are slow to render on a busy machine.
vi.setConfig({ testTimeout: 30000 });

beforeEach(() => {
  hoisted.config = { data: { eventsEnabled: true }, isLoading: false, isError: false };
  hoisted.tokenScopes = { enforced: false, resolved: true, scopes: new Set() };
  hoisted.subscriptions = {
    data: {
      subscriptions: [subscription({ id: "s1", name: "Deploys" })],
      inherited: [
        subscription({ id: "s2", name: "Org alerts" }),
        subscription({ id: "s3", name: "Project alerts" }),
      ],
    },
    isLoading: false,
    error: undefined,
  };
  hoisted.destinations = {
    data: { destinations: [destination({ id: "d1", name: "Alerts" }), destination({ id: "d2", name: "Hook" })] },
  };
  hoisted.tabProps = {};
});

describe("EventsPage", () => {
  it("waits for the deployment settings", () => {
    hoisted.config = { data: undefined, isLoading: true, isError: false };
    renderAt("/org/acme/events");

    expect(screen.queryByText("Events are not enabled")).not.toBeInTheDocument();
    expect(screen.queryByText("subscriptions tab")).not.toBeInTheDocument();
  });

  it("offers a reload when the deployment settings fail to load", () => {
    hoisted.config = { data: undefined, isLoading: false, isError: true };
    renderAt("/org/acme/events");

    expect(screen.getByText("Couldn't load the deployment settings")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reload page" })).toBeInTheDocument();
  });

  it("says when events are not enabled", () => {
    hoisted.config = { data: { eventsEnabled: false }, isLoading: false, isError: false };
    renderAt("/org/acme/project/shop/events");

    expect(screen.getByText("Events are not enabled")).toBeInTheDocument();
  });

  it("keeps organization events from users without the events scopes", () => {
    hoisted.tokenScopes = { enforced: true, resolved: true, scopes: new Set(["amp:project:update"]) };
    renderAt("/org/acme/events");

    expect(screen.getByText("You don't have access to organization events")).toBeInTheDocument();
  });

  it("shows the organization's tabs with their counts", () => {
    renderAt("/org/acme/events");

    expect(
      screen.getByText(/Subscriptions created here apply to every project/),
    ).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Subscriptions 1" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("tab", { name: "Destinations 2" })).toBeInTheDocument();
    expect(screen.getByText("subscriptions tab")).toBeInTheDocument();
    expect(hoisted.tabProps.subscriptions).toMatchObject({
      canManage: true,
      levelNoun: "organization",
      orgEventsHref: "/org/acme/events",
      projectEventsHref: undefined,
    });

    fireEvent.click(screen.getByRole("tab", { name: "Destinations 2" }));
    expect(screen.getByText("destinations tab")).toBeInTheDocument();
    expect(where()).toContain("/org/acme/events?tab=destinations");
  });

  it("opens the subscription and destination editors, coming back to the same tab", () => {
    renderAt("/org/acme/events?tab=destinations");

    fireEvent.click(screen.getByRole("button", { name: "add" }));
    expect(screen.getByText("destination editor, back to /org/acme/events")).toBeInTheDocument();
    expect(where()).toBe(
      '/org/acme/events/destinations/new {"from":"/org/acme/events?tab=destinations"}',
    );
  });

  it("opens a destination for editing", () => {
    renderAt("/org/acme/events?tab=destinations");

    fireEvent.click(screen.getByRole("button", { name: "edit" }));
    expect(where()).toContain("/org/acme/events/destinations/d1/edit");
  });

  it("opens a subscription for creating and editing", () => {
    renderAt("/org/acme/project/shop/events");

    fireEvent.click(screen.getByRole("button", { name: "new" }));
    expect(screen.getByText("subscription editor for shop, back to /org/acme/project/shop/events")).toBeInTheDocument();
  });

  it("opens an existing subscription", () => {
    renderAt("/org/acme/project/shop/agents/bot/events");

    fireEvent.click(screen.getByRole("button", { name: "edit" }));
    expect(where()).toContain("/org/acme/project/shop/agents/bot/events/subscriptions/s1/edit");
    expect(screen.getByText(/subscription editor for bot/)).toBeInTheDocument();
  });

  it("filters the delivery log to a subscription", () => {
    renderAt("/org/acme/events");

    fireEvent.click(screen.getByRole("button", { name: "deliveries" }));

    expect(screen.getByText("delivery log tab")).toBeInTheDocument();
    expect(where()).toContain("tab=deliveries&subscriptionId=s1");
    expect(hoisted.tabProps.deliveries).toMatchObject({
      subscriptionId: "s1",
      subscriptionName: "Deploys",
      destinationOptions: [
        { id: "d1", name: "Alerts" },
        { id: "d2", name: "Hook" },
      ],
    });

    fireEvent.click(screen.getByRole("button", { name: "pick destination" }));
    expect(where()).toContain("destinationId=d2");
    fireEvent.click(screen.getByRole("button", { name: "clear subscription" }));
    expect(where()).not.toContain("subscriptionId");
  });

  it("filters the delivery log to a destination", () => {
    renderAt("/org/acme/events?tab=destinations");

    fireEvent.click(screen.getByRole("button", { name: "deliveries" }));

    expect(where()).toContain("tab=deliveries&destinationId=d1");
  });

  it("shows a project's own and inherited subscription counts, without destinations", () => {
    renderAt("/org/acme/project/shop/events?tab=destinations");

    expect(screen.getByText("Event subscriptions for project", { exact: false })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Subscriptions 1 + 2 inherited" })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: /Destinations/ })).not.toBeInTheDocument();
    // An unknown tab falls back to subscriptions.
    expect(screen.getByText("subscriptions tab")).toBeInTheDocument();
    expect(hoisted.tabProps.subscriptions).toMatchObject({ levelNoun: "project" });
  });

  it("links an agent's inherited subscriptions to the project and organization", () => {
    hoisted.tokenScopes = { enforced: true, resolved: true, scopes: new Set(["amp:agent:update"]) };
    renderAt("/org/acme/project/shop/agents/bot/events?tab=deliveries");

    expect(screen.getByText("delivery log tab")).toBeInTheDocument();
    expect(hoisted.tabProps.deliveries).toMatchObject({ destinationOptions: undefined });

    fireEvent.click(screen.getByRole("tab", { name: /Subscriptions/ }));
    expect(hoisted.tabProps.subscriptions).toMatchObject({
      canManage: true,
      levelNoun: "agent",
      projectEventsHref: "/org/acme/project/shop/events",
      orgEventsHref: undefined,
    });
  });

  it("has no destination editor below a project", () => {
    renderAt("/org/acme/project/shop/events/destinations/new");

    expect(screen.queryByText(/destination editor/)).not.toBeInTheDocument();
  });
});

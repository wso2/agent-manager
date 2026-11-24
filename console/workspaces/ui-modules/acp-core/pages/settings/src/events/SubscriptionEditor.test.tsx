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
import type { EventScope, EventTargetParams } from "@agent-management-platform/types";
import { destination, kinds, subscription } from "./__test__/fixtures";
import type { EventsAccess } from "./useEventsAccess";

const hoisted = vi.hoisted(() => ({
  list: { data: undefined as unknown, isLoading: false },
  create: vi.fn(),
  update: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListEventSubscriptions: () => hoisted.list,
  useEventTypes: () => ({
    data: {
      eventTypes: [
        { type: "agent.deployed", scope: "agent", category: "Deployments", description: "An agent was deployed." },
        { type: "agent.build.failed", scope: "agent", category: "Builds", description: "A build failed." },
      ],
    },
    isLoading: false,
  }),
  useDestinationKinds: () => ({ data: kinds }),
  useListEventDestinations: () => ({
    data: {
      destinations: [
        destination({ id: "d1", name: "Platform alerts", description: "acme.slack.com" }),
        destination({ id: "d2", name: "On call", service: "pagerduty", availability: "org" }),
      ],
    },
    isLoading: false,
  }),
  useAvailableEventDestinations: () => ({
    data: {
      destinations: [
        { id: "d1", name: "Platform alerts", kind: "notify", service: "slack", health: "healthy" },
        { id: "d2", name: "On call", kind: "notify", service: "pagerduty", health: "idle" },
      ],
    },
    isLoading: false,
  }),
  useListEnvironments: () => ({
    data: [
      { name: "dev", displayName: "Development" },
      { name: "prod", displayName: "Production" },
    ],
  }),
  useCreateEventSubscription: () => ({ mutate: hoisted.create, isPending: false }),
  useUpdateEventSubscription: () => ({ mutate: hoisted.update, isPending: false }),
}));
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/shared-component")>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (go: () => void) => go() }),
}));

import { SubscriptionEditor } from "./SubscriptionEditor";

const fullAccess: EventsAccess = {
  resolved: true,
  canReadOrg: true,
  canManageDestinations: true,
  canManageSubscriptions: true,
};

const Elsewhere = () => {
  const location = useLocation();
  return (
    <div>
      <div>{`at ${location.pathname}${location.search}`}</div>
      <div data-testid="state">{JSON.stringify(location.state)}</div>
    </div>
  );
};

const bases: Record<EventScope, string> = {
  org: "/org/acme/events",
  project: "/org/acme/project/shop/events",
  agent: "/org/acme/project/shop/agents/bot/events",
};
const targets: Record<EventScope, EventTargetParams> = {
  org: { scope: "org", orgName: "acme" },
  project: { scope: "project", orgName: "acme", projName: "shop" },
  agent: { scope: "agent", orgName: "acme", projName: "shop", agentName: "bot" },
};

const renderEditor = (
  scope: EventScope,
  sub = "subscriptions/new",
  { access = fullAccess, state }: { access?: EventsAccess; state?: unknown } = {},
) => {
  const base = bases[scope];
  return render(
    <MemoryRouter
      initialEntries={[{ pathname: `${base}/${sub}`, state }]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        {[`${base}/subscriptions/new`, `${base}/subscriptions/:subscriptionId/edit`].map((p) => (
          <Route
            key={p}
            path={p}
            element={
              <SubscriptionEditor
                target={targets[scope]}
                access={access}
                listPath={base}
                levelName={scope === "org" ? "acme" : scope === "project" ? "shop" : "bot"}
              />
            }
          />
        ))}
        <Route path="*" element={<Elsewhere />} />
      </Routes>
    </MemoryRouter>,
  );
};

// The events picker cannot be opened: TextInput replaces the Autocomplete's
// InputProps (its anchor ref and adornments), so no listbox ever renders.
// Event types are brought in through a returned draft instead.
const withEvents = (eventTypes: string[], extra: Record<string, unknown> = {}) => ({
  state: {
    subscriptionDraft: {
      name: "",
      description: "",
      eventTypes,
      envMode: "all",
      envs: [],
      enabled: true,
      ...extra,
    },
  },
});
const pickDestination = (name: string) => {
  fireEvent.click(screen.getByRole("button", { name: /Select destination|Change/ }));
  fireEvent.click(screen.getByRole("button", { name: `${name}. Click to select` }));
};
const submit = (name = "Create subscription") =>
  fireEvent.click(screen.getByRole("button", { name }));

// Full MUI pages are slow to render on a busy machine.
vi.setConfig({ testTimeout: 30000 });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.list = { data: { subscriptions: [], inherited: [] }, isLoading: false };
});

describe("SubscriptionEditor", () => {
  it("asks for a name, events and a destination", () => {
    renderEditor("project");

    expect(screen.getByText("Choose which of shop's events to send, and where.")).toBeInTheDocument();
    submit();

    expect(screen.getByText("Enter a name")).toBeInTheDocument();
    expect(screen.getByText("Choose at least one event")).toBeInTheDocument();
    expect(screen.getByText("Choose where the events go")).toBeInTheDocument();
    expect(hoisted.create).not.toHaveBeenCalled();
  });

  it("creates a project subscription filtered to an environment", () => {
    hoisted.create.mockImplementation((_body, { onSuccess }) => onSuccess());
    renderEditor("project", "subscriptions/new", withEvents(["agent.deployed", "agent.build.failed"]));

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: " Deploys " } });
    fireEvent.change(screen.getByLabelText("Description (optional)"), {
      target: { value: "To the team" },
    });
    pickDestination("Platform alerts");
    expect(screen.getAllByText("Slack").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByLabelText("Selected environments"));
    submit();
    expect(screen.getByText("Select at least one environment")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Production" }));
    fireEvent.click(screen.getByRole("switch"));
    expect(screen.getByText("Disabled: nothing is sent")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Send agent.deployed and 1 more for shop in Production to Platform alerts, once enabled.",
    );
    submit();

    expect(hoisted.create).toHaveBeenCalledWith(
      {
        name: "Deploys",
        description: "To the team",
        destinationId: "d1",
        eventTypes: ["agent.deployed", "agent.build.failed"],
        environments: ["prod"],
        enabled: false,
      },
      expect.anything(),
    );
    expect(screen.getByText("at /org/acme/project/shop/events?tab=subscriptions")).toBeInTheDocument();
  });

  it("warns before paging on every event", () => {
    renderEditor("org", "subscriptions/new", withEvents(["*"]));

    expect(
      screen.getByText("Organization subscriptions send the events of every project and agent."),
    ).toBeInTheDocument();
    pickDestination("Platform alerts");
    expect(screen.queryByText(/pages people/)).not.toBeInTheDocument();
    expect(screen.getByText(/every event/)).toBeInTheDocument();

    pickDestination("On call");
    expect(screen.getByText(/PagerDuty pages people/)).toBeInTheDocument();
    expect(screen.getAllByText("Available to Org only", { exact: false }).length).toBeGreaterThan(0);
    expect(screen.getByText(/anywhere in the organization/)).toBeInTheDocument();
  });

  it("edits an existing subscription", () => {
    hoisted.list = {
      data: {
        subscriptions: [
          subscription({ id: "s1", name: "Deploys", environments: ["dev"], eventTypes: ["agent.deployed", "gone.event"] }),
        ],
        inherited: [],
      },
      isLoading: false,
    };
    hoisted.update.mockImplementation((_req, { onSuccess }) => onSuccess());
    renderEditor("agent", "subscriptions/s1/edit");

    expect(screen.getByText("Edit subscription")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toHaveValue("Deploys");
    expect(screen.getByLabelText("Selected environments")).toBeChecked();
    expect(screen.getAllByText("Platform alerts").length).toBeGreaterThan(0);

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Deploys v2" } });
    submit("Save");

    expect(hoisted.update).toHaveBeenCalledWith(
      {
        id: "s1",
        body: {
          name: "Deploys v2",
          description: "Every deploy",
          destinationId: "d1",
          eventTypes: ["agent.deployed", "gone.event"],
          environments: ["dev"],
          enabled: true,
        },
      },
      expect.anything(),
    );
    expect(
      screen.getByText("at /org/acme/project/shop/agents/bot/events?tab=subscriptions"),
    ).toBeInTheDocument();
  });

  it("says when the subscription is not found", () => {
    renderEditor("org", "subscriptions/missing/edit");

    expect(screen.getByText("Subscription not found")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Back to Events" }));
    expect(screen.getByText("at /org/acme/events?tab=subscriptions")).toBeInTheDocument();
  });

  it("waits for the list before deciding a subscription is missing", () => {
    hoisted.list = { data: undefined, isLoading: true };
    renderEditor("org", "subscriptions/s1/edit");

    expect(screen.getByText("Edit subscription")).toBeInTheDocument();
    expect(screen.queryByText("Subscription not found")).not.toBeInTheDocument();
  });

  it("takes the draft to the destination page when adding a destination", () => {
    renderEditor("project");

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Builds" } });
    fireEvent.click(screen.getByRole("button", { name: "Select destination" }));
    fireEvent.click(screen.getByRole("button", { name: "Add destination" }));

    expect(screen.getByText("at /org/acme/events/destinations/new")).toBeInTheDocument();
    const state = JSON.parse(screen.getByTestId("state").textContent!);
    expect(state).toMatchObject({
      from: "/org/acme/project/shop/events/subscriptions/new",
      defaultAvailability: { availability: "projects", projects: ["shop"] },
      subscriptionDraft: {
        name: "Builds",
        description: "",
        eventTypes: [],
        envMode: "all",
        envs: [],
        enabled: true,
        from: "/org/acme/project/shop/events?tab=subscriptions",
      },
    });
  });

  it("restores a draft brought back from the destination page", () => {
    renderEditor("project", "subscriptions/new", {
      state: {
        subscriptionDraft: {
          name: "Builds",
          description: "From draft",
          eventTypes: ["agent.build.failed"],
          destinationId: "d2",
          envMode: "selected",
          envs: ["dev"],
          enabled: true,
        },
      },
    });

    expect(screen.getByLabelText("Name")).toHaveValue("Builds");
    expect(screen.getByLabelText("Description (optional)")).toHaveValue("From draft");
    expect(screen.getAllByText("On call").length).toBeGreaterThan(0);
    expect(screen.getByLabelText("Selected environments")).toBeChecked();
  });

  it("goes back on Cancel", () => {
    renderEditor("org");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByText("at /org/acme/events?tab=subscriptions")).toBeInTheDocument();
  });

  it("blocks saving without manage access and hides adding destinations", () => {
    renderEditor("project", "subscriptions/new", {
      access: { ...fullAccess, canManageDestinations: false, canManageSubscriptions: false },
    });

    expect(screen.getByRole("button", { name: "Create subscription" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Select destination" }));
    expect(screen.getByText("Only organization admins can add destinations.")).toBeInTheDocument();
  });
});

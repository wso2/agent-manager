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
import { destination, kinds } from "./__test__/fixtures";
import type { EventsAccess } from "./useEventsAccess";

const hoisted = vi.hoisted(() => {
  // The console sets its runtime config before any module loads.
  (window as unknown as { __RUNTIME_CONFIG__: unknown }).__RUNTIME_CONFIG__ = {
    docsUrl: "https://docs.example.com",
  };
  return {
  existing: {
    data: undefined as unknown,
    isLoading: false,
    isError: false,
    error: undefined as unknown,
    isFetchedAfterMount: false,
  },
  create: vi.fn(),
  update: vi.fn(),
  };
});

vi.mock("@agent-management-platform/api-client", () => ({
  useEventDestination: () => hoisted.existing,
  useCreateEventDestination: () => ({ mutate: hoisted.create, isPending: false }),
  useUpdateEventDestination: () => ({ mutate: hoisted.update, isPending: false }),
  useDestinationKinds: () => ({ data: kinds }),
  useListAllProjects: () => ({ data: { projects: [] } }),
}));
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/shared-component")>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (go: () => void) => go() }),
}));

import { DestinationEditor } from "./DestinationEditor";

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

const renderEditor = (
  path: string,
  { access = fullAccess, state }: { access?: EventsAccess; state?: unknown } = {},
) =>
  render(
    <MemoryRouter
      initialEntries={[{ pathname: path, state }]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        {["/org/:orgId/events/destinations/new", "/org/:orgId/events/destinations/:destinationId/edit"].map(
          (p) => (
            <Route
              key={p}
              path={p}
              element={
                <DestinationEditor orgName="acme" access={access} listPath="/org/acme/events" />
              }
            />
          ),
        )}
        <Route path="*" element={<Elsewhere />} />
      </Routes>
    </MemoryRouter>,
  );

const fillWebhook = () => {
  fireEvent.click(screen.getByRole("button", { name: "Webhook" }));
  fireEvent.change(screen.getByLabelText("Endpoint URL"), {
    target: { value: "https://ci.example.com/hook" },
  });
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: "CI" } });
};

// Full MUI pages are slow to render on a busy machine.
vi.setConfig({ testTimeout: 30000 });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.existing = {
    data: undefined,
    isLoading: false,
    isError: false,
    error: undefined,
    isFetchedAfterMount: false,
  };
});

describe("DestinationEditor", () => {
  it("sends users who cannot manage destinations back to the list", () => {
    renderEditor("/org/acme/events/destinations/new", {
      access: { ...fullAccess, canManageDestinations: false },
    });

    expect(screen.getByText("at /org/acme/events?tab=destinations")).toBeInTheDocument();
  });

  it("adds a webhook and shows its signing secret once before leaving", () => {
    hoisted.create.mockImplementation((_body, { onSuccess }) =>
      onSuccess(destination({ id: "d9", name: "CI", kind: "webhook", secret: "whsec_1" })),
    );
    renderEditor("/org/acme/events/destinations/new");

    fillWebhook();
    fireEvent.click(screen.getByRole("button", { name: "Add destination" }));

    expect(hoisted.create).toHaveBeenCalledWith(
      {
        name: "CI",
        description: "",
        kind: "webhook",
        config: { url: "https://ci.example.com/hook" },
        availability: "all",
        projects: [],
      },
      expect.anything(),
    );
    expect(screen.getByText("whsec_1")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.getByText("at /org/acme/events?tab=destinations")).toBeInTheDocument();
  });

  it("returns to the subscription being edited with the new destination selected", () => {
    hoisted.create.mockImplementation((_body, { onSuccess }) =>
      onSuccess(destination({ id: "d9", name: "CI", kind: "webhook" })),
    );
    const draft = {
      name: "Builds",
      description: "",
      eventTypes: ["agent.built"],
      envMode: "all",
      envs: [],
      enabled: true,
      from: "/org/acme/events",
    };
    renderEditor("/org/acme/events/destinations/new", {
      state: {
        from: "/org/acme/events/subscriptions/new",
        subscriptionDraft: draft,
        defaultAvailability: { availability: "org", projects: [] },
      },
    });

    expect(
      screen.getByText("The new destination is selected for the subscription you were editing."),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Organization only")).toBeChecked();
    fillWebhook();
    fireEvent.click(screen.getByRole("button", { name: "Add & use" }));

    expect(screen.getByText("at /org/acme/events/subscriptions/new")).toBeInTheDocument();
    expect(JSON.parse(screen.getByTestId("state").textContent!)).toEqual({
      from: "/org/acme/events",
      subscriptionDraft: { ...draft, destinationId: "d9" },
    });
  });

  it("cancels back to the list", () => {
    renderEditor("/org/acme/events/destinations/new");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByText("at /org/acme/events?tab=destinations")).toBeInTheDocument();
    expect(hoisted.create).not.toHaveBeenCalled();
  });

  it("waits for the destination being edited", () => {
    hoisted.existing = { ...hoisted.existing, isLoading: true };
    renderEditor("/org/acme/events/destinations/d1/edit");

    expect(screen.getByText("Edit destination")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("says when the destination is gone", () => {
    renderEditor("/org/acme/events/destinations/d1/edit");

    expect(screen.getByText("Destination not found")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Back to destinations" }));
    expect(screen.getByText("at /org/acme/events?tab=destinations")).toBeInTheDocument();
  });

  it("says when the destination failed to load", () => {
    hoisted.existing = { ...hoisted.existing, isError: true, error: new Error("500") };
    renderEditor("/org/acme/events/destinations/d1/edit");

    expect(screen.getByText("Couldn't load the destination")).toBeInTheDocument();
  });

  it("saves an edited destination and goes back", () => {
    const existing = destination({ id: "d1", name: "Platform alerts" });
    hoisted.existing = { ...hoisted.existing, data: existing, isFetchedAfterMount: true };
    hoisted.update.mockImplementation((_req, { onSuccess }) => onSuccess());
    renderEditor("/org/acme/events/destinations/d1/edit");

    expect(screen.getByText("Edit Platform alerts")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Alerts" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    expect(hoisted.update).toHaveBeenCalledWith(
      {
        id: "d1",
        body: {
          name: "Alerts",
          description: "",
          kind: "notify",
          config: { service: "slack" },
          availability: "all",
          projects: [],
        },
      },
      expect.anything(),
    );
    expect(screen.getByText("at /org/acme/events?tab=destinations")).toBeInTheDocument();
  });
});

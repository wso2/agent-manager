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
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { EventSubscriptionListResponse } from "@agent-management-platform/types";
import { subscription } from "./__test__/fixtures";

const hoisted = vi.hoisted(() => ({
  update: vi.fn(),
  remove: vi.fn(),
  addConfirmation: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useUpdateEventSubscription: () => ({
    mutate: hoisted.update,
    isPending: false,
    variables: undefined,
  }),
  useDeleteEventSubscription: () => ({ mutate: hoisted.remove }),
  useUserDisplayName: ({ userId }: { userId: string }) =>
    userId === "u1" ? "nuwan.p" : undefined,
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/shared-component")>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));

import { SubscriptionsTab, type SubscriptionsTabProps } from "./SubscriptionsTab";

const own = subscription({
  id: "s1",
  name: "Deploys to Slack",
  eventTypes: ["agent.deployed", "agent.build.failed", "agent.deleted"],
  environments: ["prod"],
  last24h: { delivered: 6, failed: 0, pending: 0, skipped: 0 },
});
const fromOrg = subscription({
  id: "s2",
  scope: "org",
  name: "Org wide alerts",
  destination: { id: "d2", name: "Ops pager", kind: "notify", service: "pagerduty", health: "failing" },
  eventTypes: ["*"],
  last24h: { delivered: 0, failed: 2, pending: 0, skipped: 0 },
});
const fromProject = subscription({
  id: "s3",
  scope: "project",
  name: "Project hooks",
  destination: { id: "d3", name: "CI hook", kind: "webhook", health: "idle" },
  last24h: { delivered: 0, failed: 0, pending: 3, skipped: 0 },
});

const props = (over: Partial<SubscriptionsTabProps> = {}): SubscriptionsTabProps => ({
  target: { scope: "agent", orgName: "acme", projName: "shop", agentName: "bot" },
  data: { subscriptions: [own], inherited: [fromProject, fromOrg] },
  isLoading: false,
  error: undefined,
  canManage: true,
  levelNoun: "agent",
  projectEventsHref: "/org/acme/project/shop/events",
  orgEventsHref: "/org/acme/events",
  onNew: vi.fn(),
  onEdit: vi.fn(),
  onViewDeliveries: vi.fn(),
  ...over,
});

// MUI 7.0's Switch drops inputProps, so the switch has no aria-label; find it by row.
const switchOf = (name: string) =>
  within(screen.getByText(name).closest("tr")!).getByRole("switch");

const renderTab = (p: SubscriptionsTabProps) =>
  render(
    <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <SubscriptionsTab {...p} />
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SubscriptionsTab", () => {
  it("lists own subscriptions and groups inherited ones with links to where they are managed", () => {
    renderTab(props());

    expect(screen.getByText("This agent")).toBeInTheDocument();
    expect(screen.getByText("Deploys to Slack")).toBeInTheDocument();
    expect(screen.getByText(/^by nuwan\.p, /)).toBeInTheDocument();
    expect(screen.getByText("env: prod")).toBeInTheDocument();
    expect(screen.getByText("6 sent")).toBeInTheDocument();
    expect(screen.getByText("+1")).toBeInTheDocument();

    expect(screen.getByText("Inherited from project shop")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Manage in project" })).toHaveAttribute(
      "href",
      "/org/acme/project/shop/events",
    );
    expect(screen.getByText("From project")).toBeInTheDocument();
    expect(screen.getByText("3 pending")).toBeInTheDocument();

    expect(screen.getByText("Inherited from organization acme")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View in org" })).toHaveAttribute(
      "href",
      "/org/acme/events",
    );
    expect(screen.getByText("all events")).toBeInTheDocument();
    expect(screen.getByText("2 failed")).toBeInTheDocument();
    // Inherited rows are locked.
    expect(switchOf("Org wide alerts")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Actions for Org wide alerts" })).not.toBeInTheDocument();
  });

  it("toggles a subscription with its full body", () => {
    renderTab(props());

    fireEvent.click(switchOf("Deploys to Slack"));

    expect(hoisted.update).toHaveBeenCalledWith({
      id: "s1",
      body: {
        name: "Deploys to Slack",
        description: "Every deploy",
        destinationId: "d1",
        eventTypes: ["agent.deployed", "agent.build.failed", "agent.deleted"],
        environments: ["prod"],
        enabled: false,
      },
    });
  });

  it("opens the editor from a row and from the actions menu", () => {
    const p = props();
    renderTab(p);

    fireEvent.click(screen.getByText("Deploys to Slack"));
    expect(p.onEdit).toHaveBeenCalledWith(own);

    fireEvent.click(screen.getByRole("button", { name: "Actions for Deploys to Slack" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Edit" }));
    expect(p.onEdit).toHaveBeenCalledTimes(2);
  });

  it("opens the delivery log for a subscription", () => {
    const p = props();
    renderTab(p);

    fireEvent.click(screen.getByRole("button", { name: "Actions for Deploys to Slack" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "View deliveries" }));

    expect(p.onViewDeliveries).toHaveBeenCalledWith("s1");
  });

  it("confirms before deleting a subscription", () => {
    renderTab(props());

    fireEvent.click(screen.getByRole("button", { name: "Actions for Deploys to Slack" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    expect(hoisted.addConfirmation).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Delete subscription",
        description:
          '"Deploys to Slack" stops sending events to Platform alerts. Events already queued are still sent.',
      }),
    );
    expect(hoisted.remove).not.toHaveBeenCalled();
    hoisted.addConfirmation.mock.calls[0][0].onConfirm();
    expect(hoisted.remove).toHaveBeenCalledWith("s1");
  });

  it("searches by name, destination and event type", () => {
    renderTab(props());

    fireEvent.change(screen.getByPlaceholderText("Search subscriptions"), {
      target: { value: "ci hook" },
    });
    expect(screen.getByText("Project hooks")).toBeInTheDocument();
    expect(screen.queryByText("Deploys to Slack")).not.toBeInTheDocument();
    expect(screen.queryByText("Org wide alerts")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search subscriptions"), {
      target: { value: "nothing-like-this" },
    });
    expect(screen.getByText("No subscriptions match")).toBeInTheDocument();
  });

  it("shows only this level's or only inherited subscriptions", () => {
    renderTab(props());

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(screen.getByRole("option", { name: "Inherited" }));
    expect(screen.queryByText("Deploys to Slack")).not.toBeInTheDocument();
    expect(screen.getByText("Org wide alerts")).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(screen.getByRole("option", { name: "This agent" }));
    expect(screen.getByText("Deploys to Slack")).toBeInTheDocument();
    expect(screen.queryByText("Org wide alerts")).not.toBeInTheDocument();
  });

  it("offers no create, edit or delete without manage access", () => {
    const p = props({ canManage: false });
    renderTab(p);

    expect(screen.queryByRole("button", { name: "New subscription" })).not.toBeInTheDocument();
    expect(switchOf("Deploys to Slack")).toBeDisabled();
    fireEvent.click(screen.getByText("Deploys to Slack"));
    expect(p.onEdit).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Actions for Deploys to Slack" }));
    const menu = screen.getByRole("menu");
    expect(within(menu).queryByRole("menuitem", { name: "Edit" })).not.toBeInTheDocument();
    expect(within(menu).queryByRole("menuitem", { name: "Delete" })).not.toBeInTheDocument();
    expect(within(menu).getByRole("menuitem", { name: "View deliveries" })).toBeInTheDocument();
  });

  it("shows the empty state with a create button", () => {
    const data: EventSubscriptionListResponse = { subscriptions: [], inherited: [] };
    const p = props({ data });
    renderTab(p);

    expect(screen.getByText("No subscriptions yet")).toBeInTheDocument();
    expect(
      screen.getByText(/send this agent's events to Slack, PagerDuty or a webhook/),
    ).toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: "New subscription" })[0]);
    expect(p.onNew).toHaveBeenCalled();
  });

  it("shows a load error", () => {
    renderTab(props({ data: undefined, error: new Error("boom") }));

    expect(
      screen.getByText("Couldn't load subscriptions. Reload the page to try again."),
    ).toBeInTheDocument();
  });

  it("names an unknown creator and a system-made subscription", () => {
    renderTab(
      props({
        data: {
          subscriptions: [
            subscription({ id: "a", name: "By someone", createdBy: "u9" }),
            subscription({ id: "b", name: "By system", createdBy: "" }),
          ],
          inherited: [],
        },
      }),
    );

    expect(screen.getByText(/^by a teammate, /)).toBeInTheDocument();
    expect(screen.getByText(/^added /)).toBeInTheDocument();
    // With nothing inherited there is no group header and no Show filter.
    expect(screen.queryByText("This agent")).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  });
});

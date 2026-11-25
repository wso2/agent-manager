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

import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { destination, kinds } from "./__test__/fixtures";

const hoisted = vi.hoisted(() => ({
  list: { data: undefined as unknown, isLoading: false, error: undefined as unknown },
  single: { data: undefined as unknown, isLoading: false },
  remove: vi.fn(),
  rotate: vi.fn(),
  sendTest: vi.fn(),
  addConfirmation: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListEventDestinations: () => hoisted.list,
  useDestinationKinds: () => ({ data: kinds }),
  useEventDestination: () => hoisted.single,
  useDeleteEventDestination: () => ({ mutate: hoisted.remove }),
  useRotateEventDestinationSecret: () => ({ mutate: hoisted.rotate }),
  useTestEventDestination: () => ({ mutate: hoisted.sendTest, isPending: false }),
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));
vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@agent-management-platform/shared-component")>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));

import { DestinationsTab, type DestinationsTabProps } from "./DestinationsTab";

const slack = destination({
  id: "d1",
  name: "Platform alerts",
  usedBy: 1,
  availability: "all",
  lastDeliveryAt: undefined,
});
const hook = destination({
  id: "d2",
  name: "CI hook",
  kind: "webhook",
  service: undefined,
  config: { url: "https://ci.example.com/hook" },
  availability: "projects",
  projects: ["shop", "pay", "billing"],
  usedBy: 3,
  health: "failing",
  consecutiveFailures: 4,
  lastDeliveryAt: "2026-01-01T00:00:00Z",
});
const pager = destination({
  id: "d3",
  name: "On call",
  service: "pagerduty",
  description: "Primary rotation",
  availability: "org",
  usedBy: 0,
  health: "idle",
  lastDeliveryAt: "2026-01-01T00:00:00Z",
});

const props = (over: Partial<DestinationsTabProps> = {}): DestinationsTabProps => ({
  orgName: "acme",
  canManage: true,
  onAdd: vi.fn(),
  onEdit: vi.fn(),
  onViewDeliveries: vi.fn(),
  ...over,
});

const openActions = (name: string) =>
  fireEvent.click(screen.getByRole("button", { name: `Actions for ${name}` }));

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.list = {
    data: { destinations: [slack, hook, pager] },
    isLoading: false,
    error: undefined,
  };
  hoisted.single = { data: undefined, isLoading: false };
});

describe("DestinationsTab", () => {
  it("lists destinations with type, availability, usage and health", () => {
    render(<DestinationsTab {...props()} />);

    expect(screen.getByText("Platform alerts")).toBeInTheDocument();
    expect(screen.getByText("Slack service")).toBeInTheDocument();
    expect(screen.getByText("All projects")).toBeInTheDocument();
    expect(screen.getByText("1 subscription")).toBeInTheDocument();
    expect(screen.getByText("never")).toBeInTheDocument();

    expect(screen.getByText("https://ci.example.com/hook")).toBeInTheDocument();
    expect(screen.getByText("shop, pay +1")).toBeInTheDocument();
    expect(screen.getByText("3 subscriptions")).toBeInTheDocument();
    expect(screen.getByText("Failing, 4 deliveries")).toBeInTheDocument();

    expect(screen.getByText("Primary rotation")).toBeInTheDocument();
    expect(screen.getByText("Org only")).toBeInTheDocument();
    expect(screen.getByText("No deliveries")).toBeInTheDocument();
  });

  it("searches and filters by type", () => {
    render(<DestinationsTab {...props()} />);

    fireEvent.change(screen.getByPlaceholderText("Search destinations"), {
      target: { value: "rotation" },
    });
    expect(screen.getByText("On call")).toBeInTheDocument();
    expect(screen.queryByText("Platform alerts")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search destinations"), {
      target: { value: "" },
    });
    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(screen.getByRole("option", { name: "Webhook" }));
    expect(screen.getByText("CI hook")).toBeInTheDocument();
    expect(screen.queryByText("Platform alerts")).not.toBeInTheDocument();
    expect(screen.queryByText("On call")).not.toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText("Search destinations"), {
      target: { value: "slack" },
    });
    expect(screen.getByText("No destinations match your filters")).toBeInTheDocument();
  });

  it("opens the editor from a row and adds from the toolbar", () => {
    const p = props();
    render(<DestinationsTab {...p} />);

    fireEvent.click(screen.getByText("Platform alerts"));
    expect(p.onEdit).toHaveBeenCalledWith(slack);

    fireEvent.click(screen.getByRole("button", { name: "Add destination" }));
    expect(p.onAdd).toHaveBeenCalled();
  });

  it("confirms before deleting an unused destination", () => {
    render(<DestinationsTab {...props()} />);

    openActions("On call");
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    expect(hoisted.addConfirmation).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Delete destination" }),
    );
    hoisted.addConfirmation.mock.calls[0][0].onConfirm();
    expect(hoisted.remove).toHaveBeenCalledWith("d3");
  });

  it("lists who uses a destination instead of deleting it", () => {
    hoisted.single = {
      data: {
        ...hook,
        subscriptions: [
          { id: "s1", name: "Builds", scope: "project", projectName: "shop" },
          { id: "s2", name: "Bot deploys", scope: "agent", projectName: "shop", agentName: "bot" },
          { id: "s3", name: "Everything", scope: "org" },
        ],
      },
      isLoading: false,
    };
    render(<DestinationsTab {...props()} />);

    openActions("CI hook");
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    expect(hoisted.addConfirmation).not.toHaveBeenCalled();
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("CI hook is in use")).toBeInTheDocument();
    expect(within(dialog).getByText("(project shop)")).toBeInTheDocument();
    expect(within(dialog).getByText("(agent shop/bot)")).toBeInTheDocument();
    expect(within(dialog).getByText("(organization)")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
  });

  it("sends a test event and reports the result", () => {
    hoisted.sendTest.mockImplementation((_id, { onSuccess }) =>
      onSuccess({ delivered: true, statusCode: 204 }),
    );
    render(<DestinationsTab {...props()} />);

    openActions("Platform alerts");
    fireEvent.click(screen.getByRole("menuitem", { name: "Send test event" }));

    expect(hoisted.sendTest).toHaveBeenCalledWith("d1", expect.anything());
    expect(
      screen.getByText("Test event delivered to Platform alerts (HTTP 204)."),
    ).toBeInTheDocument();
  });

  it("reports a failed test event", () => {
    hoisted.sendTest.mockImplementation((_id, { onSuccess }) =>
      onSuccess({ delivered: false, error: "connection refused" }),
    );
    render(<DestinationsTab {...props()} />);

    openActions("CI hook");
    fireEvent.click(screen.getByRole("menuitem", { name: "Send test event" }));

    expect(
      screen.getByText("Test event to CI hook failed: connection refused"),
    ).toBeInTheDocument();
  });

  it("rotates a webhook's secret after confirming and shows the new one", () => {
    hoisted.rotate.mockImplementation((_id, { onSuccess }) => onSuccess({ secret: "whsec_new" }));
    render(<DestinationsTab {...props()} />);

    openActions("Platform alerts");
    expect(
      screen.queryByRole("menuitem", { name: "Rotate signing secret" }),
    ).not.toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });

    openActions("CI hook");
    fireEvent.click(screen.getByRole("menuitem", { name: "Rotate signing secret" }));
    expect(hoisted.addConfirmation).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Rotate signing secret" }),
    );
    act(() => hoisted.addConfirmation.mock.calls[0][0].onConfirm());

    expect(hoisted.rotate).toHaveBeenCalledWith("d2", expect.anything());
    expect(screen.getByText("whsec_new")).toBeInTheDocument();
  });

  it("opens the delivery log for a destination", () => {
    const p = props();
    render(<DestinationsTab {...p} />);

    openActions("CI hook");
    fireEvent.click(screen.getByRole("menuitem", { name: "View deliveries" }));

    expect(p.onViewDeliveries).toHaveBeenCalledWith("d2");
  });

  it("only lets readers view deliveries", () => {
    const p = props({ canManage: false });
    render(<DestinationsTab {...p} />);

    expect(screen.queryByRole("button", { name: "Add destination" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Platform alerts"));
    expect(p.onEdit).not.toHaveBeenCalled();

    openActions("CI hook");
    const items = within(screen.getByRole("menu")).getAllByRole("menuitem");
    expect(items.map((i) => i.textContent)).toEqual(["View deliveries"]);
  });

  it("shows the empty state and a load error", () => {
    hoisted.list = { data: { destinations: [] }, isLoading: false, error: undefined };
    const { unmount } = render(<DestinationsTab {...props()} />);
    expect(screen.getByText("No destinations yet")).toBeInTheDocument();
    unmount();

    hoisted.list = { data: undefined, isLoading: false, error: new Error("x") };
    render(<DestinationsTab {...props()} />);
    expect(
      screen.getByText("Couldn't load destinations. Reload the page to try again."),
    ).toBeInTheDocument();
  });
});

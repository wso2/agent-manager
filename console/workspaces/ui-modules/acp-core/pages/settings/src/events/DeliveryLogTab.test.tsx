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
import { beforeEach, describe, expect, it, vi } from "vitest";
import { delivery } from "./__test__/fixtures";

const hoisted = vi.hoisted(() => ({
  result: {
    data: undefined as unknown,
    isLoading: false,
    error: undefined as unknown,
    isFetching: false,
  },
  refetch: vi.fn(),
  queries: [] as unknown[],
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListEventDeliveries: (_target: unknown, query: unknown) => {
    hoisted.queries.push(query);
    return { ...hoisted.result, refetch: hoisted.refetch };
  },
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));

import { DeliveryLogTab, type DeliveryLogTabProps } from "./DeliveryLogTab";

const props = (over: Partial<DeliveryLogTabProps> = {}): DeliveryLogTabProps => ({
  target: { scope: "org", orgName: "acme" },
  onDestinationChange: vi.fn(),
  onClearSubscription: vi.fn(),
  ...over,
});

const rows = [
  delivery(),
  delivery({
    id: "del2",
    eventId: "evt-2",
    eventType: "project.created",
    projectName: "shop",
    agentName: undefined,
    environment: undefined,
    destination: undefined,
    subscriptions: [
      { id: "a", name: "First", scope: "org" },
      { id: "b", name: "Second", scope: "project" },
    ],
    status: "failed",
    attempts: 5,
    responseCode: undefined,
    lastError: "502 Bad Gateway",
  }),
  delivery({
    id: "del3",
    eventType: "org.updated",
    projectName: undefined,
    agentName: undefined,
    environment: undefined,
    subscriptions: [],
    status: "pending",
    attempts: 0,
    responseCode: undefined,
  }),
];

const lastQuery = () => hoisted.queries.at(-1);

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.queries = [];
  hoisted.result = {
    data: { deliveries: rows, total: 60 },
    isLoading: false,
    error: undefined,
    isFetching: false,
  };
});

describe("DeliveryLogTab", () => {
  it("lists deliveries with where they came from and how they went", () => {
    render(<DeliveryLogTab {...props()} />);

    expect(screen.getByText("shop / bot · dev")).toBeInTheDocument();
    expect(screen.getByText("Deploys to Slack")).toBeInTheDocument();
    expect(screen.getByText("Delivered")).toBeInTheDocument();
    expect(screen.getByText("200")).toBeInTheDocument();

    expect(screen.getByText("shop")).toBeInTheDocument();
    expect(screen.getByText("Deleted")).toBeInTheDocument();
    expect(screen.getByText("First +1")).toBeInTheDocument();
    expect(screen.getByText("Failed")).toBeInTheDocument();

    expect(screen.getByText("Organization")).toBeInTheDocument();
    expect(screen.getByText("Queued")).toBeInTheDocument();

    expect(screen.getByText("1–25 of 60")).toBeInTheDocument();
    expect(lastQuery()).toEqual({
      destinationId: undefined,
      subscriptionId: undefined,
      status: undefined,
      limit: 25,
      offset: 0,
    });
  });

  it("pages through older and newer deliveries", () => {
    render(<DeliveryLogTab {...props()} />);

    expect(screen.getByRole("button", { name: "Newer" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(lastQuery()).toMatchObject({ offset: 25 });
    expect(screen.getByText("26–50 of 60")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(screen.getByText("51–60 of 60")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Older" })).toBeDisabled();

    fireEvent.click(screen.getByRole("button", { name: "Newer" }));
    expect(lastQuery()).toMatchObject({ offset: 25 });
  });

  it("filters by status and resets to the first page", () => {
    render(<DeliveryLogTab {...props()} />);
    fireEvent.click(screen.getByRole("button", { name: "Older" }));

    fireEvent.mouseDown(screen.getByRole("combobox"));
    fireEvent.click(screen.getByRole("option", { name: "Failed" }));

    expect(lastQuery()).toMatchObject({ status: "failed", offset: 0 });
    expect(screen.getByText("Status: Failed")).toBeInTheDocument();
  });

  it("filters by destination at organization level", () => {
    const p = props({
      destinationOptions: [
        { id: "d1", name: "Platform alerts" },
        { id: "d2", name: "CI hook" },
      ],
      destinationId: "d2",
    });
    render(<DeliveryLogTab {...p} />);

    expect(screen.getByText("Destination: CI hook")).toBeInTheDocument();
    expect(lastQuery()).toMatchObject({ destinationId: "d2" });

    fireEvent.mouseDown(screen.getAllByRole("combobox")[0]);
    fireEvent.click(screen.getByRole("option", { name: "Platform alerts" }));
    expect(p.onDestinationChange).toHaveBeenCalledWith("d1");

    fireEvent.mouseDown(screen.getAllByRole("combobox")[0]);
    fireEvent.click(screen.getByRole("option", { name: "All" }));
    expect(p.onDestinationChange).toHaveBeenLastCalledWith(undefined);
  });

  it("shows and clears a subscription filter", () => {
    const p = props({ subscriptionId: "s1", subscriptionName: "Deploys to Slack" });
    render(<DeliveryLogTab {...p} />);

    const chip = screen.getByText("Subscription: Deploys to Slack").closest(".MuiChip-root")!;
    expect(lastQuery()).toMatchObject({ subscriptionId: "s1" });
    fireEvent.click(within(chip as HTMLElement).getByTestId("CancelIcon"));
    expect(p.onClearSubscription).toHaveBeenCalled();
  });

  it("refreshes the log", () => {
    render(<DeliveryLogTab {...props()} />);

    fireEvent.click(screen.getByRole("button", { name: "Refresh delivery log" }));

    expect(hoisted.refetch).toHaveBeenCalled();
  });

  it("opens a delivery's details", () => {
    render(<DeliveryLogTab {...props()} />);

    fireEvent.click(screen.getByText("First +1"));

    expect(screen.getByText("evt-2")).toBeInTheDocument();
    expect(screen.getByText("502 Bad Gateway")).toBeInTheDocument();
    expect(screen.getByText("None")).toBeInTheDocument();
    expect(screen.getByText("(project)")).toBeInTheDocument();
  });

  it("explains an empty log and a filtered log with no matches", () => {
    hoisted.result = { ...hoisted.result, data: { deliveries: [], total: 0 } };
    const { unmount } = render(<DeliveryLogTab {...props()} />);
    expect(screen.getByText("No deliveries yet")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
    expect(hoisted.refetch).toHaveBeenCalled();
    unmount();

    render(<DeliveryLogTab {...props({ destinationId: "d1", destinationOptions: [] })} />);
    expect(screen.getByText("No deliveries match")).toBeInTheDocument();
  });

  it("shows a load error", () => {
    hoisted.result = { ...hoisted.result, data: undefined, error: new Error("x") };
    render(<DeliveryLogTab {...props()} />);

    expect(
      screen.getByText("Couldn't load the delivery log. Reload the page to try again."),
    ).toBeInTheDocument();
  });

  it("steps back to the last page when rows age out", () => {
    const { rerender } = render(<DeliveryLogTab {...props()} />);
    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    fireEvent.click(screen.getByRole("button", { name: "Older" }));
    expect(lastQuery()).toMatchObject({ offset: 50 });

    hoisted.result = { ...hoisted.result, data: { deliveries: rows, total: 30 } };
    rerender(<DeliveryLogTab {...props()} />);

    expect(lastQuery()).toMatchObject({ offset: 25 });
  });
});

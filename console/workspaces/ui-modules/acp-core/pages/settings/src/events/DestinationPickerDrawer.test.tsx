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
import { describe, expect, it, vi } from "vitest";
import { kinds } from "./__test__/fixtures";
import {
  DestinationPickerDrawer,
  type DestinationPickerDrawerProps,
  type PickableDestination,
} from "./DestinationPickerDrawer";

const destinations: PickableDestination[] = [
  {
    id: "d1",
    name: "Platform alerts",
    kind: "notify",
    service: "slack",
    health: "healthy",
    availableTo: "All projects",
    description: "acme.slack.com",
  },
  { id: "d2", name: "CI hook", kind: "webhook", health: "failing" },
];

const props = (over: Partial<DestinationPickerDrawerProps> = {}): DestinationPickerDrawerProps => ({
  open: true,
  onClose: vi.fn(),
  destinations,
  services: kinds.kinds[0].services,
  isLoading: false,
  selectedId: "d2",
  onSelect: vi.fn(),
  ...over,
});

describe("DestinationPickerDrawer", () => {
  it("lists destinations, marking the selected one", () => {
    render(<DestinationPickerDrawer {...props()} />);

    expect(screen.getByText("Available to All projects · acme.slack.com")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "CI hook. Selected" })).toBeInTheDocument();
    expect(screen.getByText("Only organization admins can add destinations.")).toBeInTheDocument();
  });

  it("selects a destination and closes", () => {
    const p = props();
    render(<DestinationPickerDrawer {...p} />);

    fireEvent.click(screen.getByRole("button", { name: "Platform alerts. Click to select" }));

    expect(p.onSelect).toHaveBeenCalledWith("d1");
    expect(p.onClose).toHaveBeenCalled();
  });

  it("searches by name, service and description", () => {
    render(<DestinationPickerDrawer {...props()} />);
    const search = screen.getByPlaceholderText("Search destinations");

    fireEvent.change(search, { target: { value: "webhook" } });
    expect(screen.getByText("CI hook")).toBeInTheDocument();
    expect(screen.queryByText("Platform alerts")).not.toBeInTheDocument();

    fireEvent.change(search, { target: { value: "acme" } });
    expect(screen.getByText("Platform alerts")).toBeInTheDocument();

    fireEvent.change(search, { target: { value: "zzz" } });
    expect(screen.getByText("No destinations match your search")).toBeInTheDocument();
  });

  it("explains an empty list depending on who can add destinations", () => {
    const onAdd = vi.fn();
    const { rerender } = render(
      <DestinationPickerDrawer {...props({ destinations: [], onAdd })} />,
    );
    expect(
      screen.getByText("Add one below, then send this subscription's events to it."),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add destination" }));
    expect(onAdd).toHaveBeenCalled();

    rerender(<DestinationPickerDrawer {...props({ destinations: [] })} />);
    expect(screen.getByText(/Ask an organization admin to add a destination/)).toBeInTheDocument();
  });

  it("shows placeholders while loading", () => {
    render(<DestinationPickerDrawer {...props({ isLoading: true })} />);

    expect(screen.queryByText("Platform alerts")).not.toBeInTheDocument();
    expect(screen.queryByText("No destinations available")).not.toBeInTheDocument();
  });
});

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const track = vi.fn();
vi.mock("@agent-management-platform/api-client", () => ({
  useTrack: () => ({ track }),
  ConsoleAction: { Search: "search", EmptyState: "empty-state" },
}));

import { ResourceListShell, type ResourceListShellProps } from "./ResourceListShell";
import { isListEmpty } from "./isListEmpty";

const emptyState = {
  illustration: <span>illus</span>,
  title: "No agents yet",
  description: "Create one to get started.",
  action: <button type="button">Create agent</button>,
};

function setup(props: Partial<ResourceListShellProps> = {}) {
  const onSearchChange = vi.fn();
  const utils = render(
    <ResourceListShell
      resourceName="agents"
      searchValue=""
      onSearchChange={onSearchChange}
      searchPlaceholder="Search agents"
      emptyState={emptyState}
      {...props}
    >
      <div>the table</div>
    </ResourceListShell>,
  );
  return { onSearchChange, ...utils };
}

describe("isListEmpty", () => {
  it.each([
    [{ count: 0 }, true],
    [{ count: 2 }, false],
    [{ count: 0, isLoading: true }, false],
    [{ count: 0, isFiltered: true }, false],
  ])("%j -> %s", (input, expected) => expect(isListEmpty(input)).toBe(expected));
});

describe("ResourceListShell", () => {
  beforeEach(() => track.mockClear());
  afterEach(() => vi.useRealTimers());

  it("renders the content with a search box and forwards searches", () => {
    const { onSearchChange } = setup();
    expect(screen.getByText("the table")).toBeInTheDocument();
    fireEvent.change(screen.getByPlaceholderText("Search agents"), { target: { value: "bot" } });
    expect(onSearchChange).toHaveBeenCalledWith("bot");
    expect(track).not.toHaveBeenCalled();
  });

  it("shows loading rows (or a custom placeholder) and keeps the toolbar", () => {
    const { container, rerender } = setup({ isLoading: true, isEmpty: true, searchDisabled: true });
    expect(container.querySelectorAll(".MuiSkeleton-root")).toHaveLength(15);
    expect(screen.getByPlaceholderText("Search agents")).toBeDisabled();
    expect(screen.queryByText("the table")).not.toBeInTheDocument();
    rerender(
      <ResourceListShell searchValue="" onSearchChange={vi.fn()} emptyState={emptyState} isLoading loadingPlaceholder={<span>custom loading</span>}>
        <div>the table</div>
      </ResourceListShell>,
    );
    expect(screen.getByText("custom loading")).toBeInTheDocument();
    expect(track).not.toHaveBeenCalled();
  });

  it.each([
    [new Error("Backend down"), "Backend down"],
    ["oops", "Failed to load. Please try again."],
  ])("shows the error %j ahead of every other state", (error, message) => {
    setup({ error, isLoading: true, isEmpty: true });
    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Search agents")).toBeInTheDocument();
    expect(track).not.toHaveBeenCalled();
  });

  it("shows the empty state without a toolbar and reports it once", () => {
    const { rerender } = setup({ isEmpty: true });
    expect(screen.getByText("No agents yet")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create agent" })).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search agents")).not.toBeInTheDocument();
    rerender(
      <ResourceListShell resourceName="agents" searchValue="" onSearchChange={vi.fn()} emptyState={emptyState} isEmpty disablePaper>
        <div />
      </ResourceListShell>,
    );
    expect(track).toHaveBeenCalledTimes(1);
    expect(track).toHaveBeenCalledWith("empty-state", { resource: "agents", state: "empty" });
  });

  it("shows the default or a custom search-empty state", () => {
    const { rerender } = setup({ isSearchEmpty: true });
    expect(screen.getByText("No results match your search")).toBeInTheDocument();
    expect(track).toHaveBeenCalledWith("empty-state", { resource: "agents", state: "search-empty" });
    rerender(
      <ResourceListShell
        searchValue=""
        onSearchChange={vi.fn()}
        emptyState={emptyState}
        isSearchEmpty
        searchEmptyState={{ illustration: null, title: "Nothing found", description: "d" }}
      >
        <div />
      </ResourceListShell>,
    );
    expect(screen.getByText("Nothing found")).toBeInTheDocument();
  });

  it("replaces the toolbar when one is given", () => {
    setup({ toolbar: <div>custom toolbar</div> });
    expect(screen.getByText("custom toolbar")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search agents")).not.toBeInTheDocument();
  });

  it("reports a search once typing settles, with only the query length", () => {
    vi.useFakeTimers();
    setup({ searchValue: "secret-name", isSearchEmpty: true });
    act(() => {
      vi.advanceTimersByTime(1199);
    });
    expect(track).not.toHaveBeenCalledWith("search", expect.anything());
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(track).toHaveBeenCalledWith("search", {
      scope: "agents",
      query_length: 11,
      zero_results: true,
    });
  });
});

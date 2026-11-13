/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { useEffect } from "react";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { ThemeProvider, createTheme } from "@wso2/oxygen-ui";
import type {
  TraceExportResponse,
  TraceFilters,
  TraceOverview,
} from "@agent-management-platform/types";

// api-client crashes at import time outside a configured app shell, and
// EnvironmentSelector pulls it in; stub both at the module boundary.
vi.mock("@agent-management-platform/api-client", () => ({
  useTraceList: vi.fn(),
  useExportTraces: vi.fn(() => ({ mutateAsync: vi.fn(), isPending: false })),
  useGetOrganization: vi.fn(() => ({
    data: { namespace: "ns" },
    isPending: false,
    isSuccess: true,
  })),
  useGetAgent: vi.fn(() => ({ data: { uuid: "agent-uuid" }, isPending: false, isSuccess: true })),
  useListEnvironments: vi.fn(() => ({
    data: [{ name: "dev" }],
    isPending: false,
    isSuccess: true,
  })),
  isObserverConfigured: vi.fn(() => true),
  useTrack: vi.fn(() => ({ track: vi.fn() })),
  ConsoleAction: { TraceOpened: "trace_opened", DownloadArtifact: "download_artifact" },
  useTrace: vi.fn(() => ({ data: undefined, isLoading: true, isTruncated: false })),
  useTraceScores: vi.fn(() => ({ data: undefined, isLoading: true })),
  useSpanDetail: vi.fn(() => ({ data: undefined, isLoading: false })),
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  EnvironmentSelector: () => null,
  copyToClipboard: vi.fn(() => Promise.resolve(true)),
}));

import { useExportTraces, useTrace, useTraceList } from "@agent-management-platform/api-client";
import { copyToClipboard } from "@agent-management-platform/shared-component";
import { TracesComponent } from "./Traces.Component";
import {
  appliedFilters,
  customError,
  isDraftValid,
  latencyInputValue,
  parseCountInput,
  parseLatencyInput,
  parseTraceFilters,
  traceFilterChips,
  withTraceFilters,
} from "./traceFilters";
import { parseTraceColumns } from "./traceColumns";
import { formatStartTime } from "./traceTime";

const mockUseTraceList = vi.mocked(useTraceList);

const makeTrace = (
  traceId: string,
  errorCount = 0,
  extra: Partial<TraceOverview> = {},
): TraceOverview => ({
  traceId,
  rootSpanId: `${traceId}-root`,
  rootSpanName: `root ${traceId}`,
  rootSpanKind: "agent",
  startTime: "2026-10-01T10:00:00Z",
  endTime: "2026-10-01T10:00:05Z",
  durationInNanos: 5_000_000_000,
  spanCount: 12,
  status: { errorCount },
  ...extra,
} as TraceOverview);

// The filtered list keeps only t-err; the unfiltered list holds both.
const ALL = [
  makeTrace("t-err", 1, { conversationId: "conv-1", models: ["gpt-4o"] }),
  makeTrace("t-ok", 0),
];
const listCache = new Map<string, unknown>();
/** Cached list data for the filters; status=error keeps only errored traces. */
const listFor = (filters: TraceFilters = {}) => {
  const key = JSON.stringify(filters);
  if (!listCache.has(key)) {
    const traces = filters.status === "error" ? ALL.filter((t) => t.status?.errorCount) : ALL;
    listCache.set(key, {
      traces,
      totalCount: traces.length,
      fetchedRange: { startTime: "2026-10-01T09:00:00Z", endTime: "2026-10-01T11:00:00Z" },
    });
  }
  return listCache.get(key);
};

const lastFilters = () => mockUseTraceList.mock.lastCall?.[9]?.filters;
const lastIncludeModels = () => mockUseTraceList.mock.lastCall?.[9]?.includeModels;

// Each URL search the page showed, in order, so tests can count updates.
const searchHistory: string[] = [];

// Hook fields a test can override, such as truncated or hasMore.
let hookOverrides: Record<string, unknown> = {};
const loadMore = vi.fn();

// jsdom has no IntersectionObserver; tests capture each observer and trigger it by hand.
const observers: { callback: IntersectionObserverCallback; targets: Element[]; live: boolean }[] =
  [];
class FakeIntersectionObserver {
  private record: (typeof observers)[number];
  constructor(callback: IntersectionObserverCallback) {
    this.record = { callback, targets: [], live: true };
    observers.push(this.record);
  }
  observe(target: Element) {
    this.record.targets.push(target);
  }
  unobserve() {}
  disconnect() {
    this.record.live = false;
  }
  takeRecords() {
    return [];
  }
}
vi.stubGlobal("IntersectionObserver", FakeIntersectionObserver);

// Reports every observed sentinel as in view.
const scrollToSentinel = () =>
  act(() => {
    for (const o of observers.filter((x) => x.live)) {
      const entries = o.targets.map((target) => ({ isIntersecting: true, target }));
      o.callback(entries as unknown as IntersectionObserverEntry[], {} as IntersectionObserver);
    }
  });

beforeEach(() => {
  vi.clearAllMocks();
  listCache.clear();
  observers.length = 0;
  searchHistory.length = 0;
  hookOverrides = {};
  mockUseTraceList.mockImplementation((...args) => ({
    data: listFor(args[9]?.filters),
    isLoading: false,
    refetch: vi.fn(),
    isRefetching: false,
    loadMore,
    isLoadingMore: false,
    hasMore: false,
    truncated: false,
    lookedBackTo: undefined,
    ...hookOverrides,
  }) as unknown as ReturnType<typeof useTraceList>);
});

/** Renders the current URL search so tests can read it, and records each one. */
function SearchProbe() {
  const location = useLocation();
  useEffect(() => {
    searchHistory.push(location.search);
  }, [location]);
  return <div data-testid="search">{location.search}</div>;
}
const currentParams = () => new URLSearchParams(screen.getByTestId("search").textContent ?? "");

/** The traces page at the given search string. */
const pageTree = (search: string) => (
    <ThemeProvider theme={createTheme()}>
      <MemoryRouter initialEntries={[`/organizations/o/projects/p/agents/a/environment/dev/traces${search}`]}>
        <Routes>
          <Route
            path="/organizations/:orgId/projects/:projectId/agents/:agentId/environment/:envId/traces"
            element={
              <>
                <TracesComponent />
                <SearchProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
);

/** Renders the traces page; rerenderPage re-reads hookOverrides, as a hook state change would. */
const renderPage = (search = "") => {
  const view = render(pageTree(search));
  return { ...view, rerenderPage: () => view.rerender(pageTree(search)) };
};

// hidden: an open trace drawer marks the page behind it aria-hidden.
const statusButton = (name: string) =>
  within(screen.getByRole("group", { name: "Status", hidden: true })).getByRole("button", {
    name,
    hidden: true,
  });
const filtersButton = () => screen.getByRole("button", { name: "Advanced filters", hidden: true });
const openFilters = () => fireEvent.click(filtersButton());
const queryDrawer = () => screen.queryByRole("complementary", { name: "Advanced Filters" });
const drawer = () => screen.getByRole("complementary", { name: "Advanced Filters" });
const pillGroup = (label: string) => within(drawer()).getByRole("group", { name: label });
const pill = (group: string, name: string) =>
  within(pillGroup(group)).getByRole("button", { name });
const pickPill = (group: string, name: string) => fireEvent.click(pill(group, name));
const pillLabels = (group: string) =>
  within(pillGroup(group)).getAllByRole("button").map((b) => b.textContent);
const pressedPill = (group: string) =>
  within(pillGroup(group))
    .getAllByRole("button")
    .filter((b) => b.getAttribute("aria-pressed") === "true")
    .map((b) => b.textContent);
const drawerField = (name: string) => within(drawer()).getByRole("textbox", { name });
const typeInto = (name: string, value: string) =>
  fireEvent.change(drawerField(name), { target: { value } });
const applyButton = () => within(drawer()).getByRole("button", { name: "Filter" });
const applyFilters = () => fireEvent.click(applyButton());
const toolFailedBox = () =>
  within(drawer()).getByRole("checkbox", { name: "Only traces where a tool call failed" });
// Chips carry their full label as the title.
const chip = (label: string) => screen.queryByTitle(label);

describe("trace filter URL parsing", () => {
  it("keeps valid params and drops ones the API would reject", () => {
    expect(
      parseTraceFilters(
        new URLSearchParams(
          "status=error&minDurationMs=5000&minTokens=5k&minSpanCount=-1&model=%20gpt-4o%20&conversationId=",
        ),
      ),
    ).toEqual({ status: "error", minDurationMs: 5000, model: "gpt-4o" });
    expect(parseTraceFilters(new URLSearchParams("status=failed&minTokens=0"))).toEqual({
      minTokens: 0,
    });
    const atLimit = "a".repeat(256);
    expect(
      parseTraceFilters(new URLSearchParams({ model: atLimit, conversationId: `${atLimit}b` })),
    ).toEqual({ model: atLimit });
  });

  it("labels chips for people", () => {
    expect(
      traceFilterChips({ minDurationMs: 5000, minTokens: 10000, minSpanCount: 20 }).map(
        (c) => c.label,
      ),
    ).toEqual(["Latency ≥ 5s", "Tokens ≥ 10k", "Steps ≥ 20"]);
  });

  it("splits each chip into a name and a value", () => {
    expect(
      traceFilterChips({
        status: "error",
        minDurationMs: 5000,
        model: "gpt-4o",
        tool: "reset_password",
      }).map((c) => [c.name, c.value]),
    ).toEqual([
      ["Status", "Error"],
      ["Latency", "≥ 5s"],
      ["Model", "gpt-4o"],
      ["Tool", "reset_password"],
    ]);
    expect(traceFilterChips({ toolError: true }).map((c) => [c.name, c.value])).toEqual([
      ["Tool", "any failed"],
    ]);
  });
});

describe("custom threshold parsing", () => {
  it("reads latency as seconds and stores whole ms", () => {
    expect(parseLatencyInput("2.5")).toBe(2500);
    expect(parseLatencyInput(" 30 ")).toBe(30000);
    expect(parseLatencyInput(".5")).toBe(500);
    expect(parseLatencyInput("0")).toBe(0);
    for (const bad of ["", "-1", "5k", "1e3", "2.5s", "1,5"]) {
      expect(parseLatencyInput(bad)).toBeUndefined();
    }
  });

  it("reads tokens and steps as whole non-negative numbers", () => {
    expect(parseCountInput("7500")).toBe(7500);
    expect(parseCountInput("0")).toBe(0);
    for (const bad of ["", "-1", "5k", "1e3", "2.5"]) {
      expect(parseCountInput(bad)).toBeUndefined();
    }
  });

  it("prefills a custom field in the field's unit", () => {
    expect(latencyInputValue(2500)).toBe("2.5");
  });

  it("applies a draft with custom values parsed, text trimmed and status kept", () => {
    expect(
      appliedFilters(
        {
          filters: { minDurationMs: 1000, model: " gpt-4o ", conversationId: "  ", toolError: false },
          custom: { minDurationMs: "2.5" },
        },
        "error",
      ),
    ).toEqual({ status: "error", minDurationMs: 2500, model: "gpt-4o" });
  });

  it("marks a custom value the field can't read as invalid", () => {
    const draft = { filters: {}, custom: { minDurationMs: "5k" } };
    expect(customError(draft, "minDurationMs")).toBe("Enter seconds, like 2.5");
    expect(isDraftValid(draft)).toBe(false);
    expect(isDraftValid({ ...draft, custom: { minDurationMs: "2.5" } })).toBe(true);
  });
});

describe("TracesComponent filters", () => {
  it("reproduces a pasted URL's filters in the request, the toggle and the chips", () => {
    renderPage("?timeRange=1h&status=error&minDurationMs=5000&model=gpt-4o&minTokens=abc");

    expect(lastFilters()).toEqual({ status: "error", minDurationMs: 5000, model: "gpt-4o" });
    expect(statusButton("Errors")).toHaveAttribute("aria-pressed", "true");
    expect(chip("Status: Error")).not.toBeInTheDocument();
    expect(chip("Latency ≥ 5s")).toBeInTheDocument();
    expect(chip("Model: gpt-4o")).toBeInTheDocument();
    expect(screen.queryByTitle(/^Tokens ≥/)).not.toBeInTheDocument();
  });

  it("writes a picked filter to the URL under the API param name", () => {
    renderPage("?timeRange=1h&limit=20");

    openFilters();
    pickPill("Latency at least", "10s");
    pickPill("Tokens at least", "5k");
    applyFilters();

    const params = currentParams();
    expect(params.get("minDurationMs")).toBe("10000");
    expect(params.get("minTokens")).toBe("5000");
    expect(params.get("timeRange")).toBe("1h");
    expect(params.get("limit")).toBe("20");
    expect(lastFilters()).toEqual({ minDurationMs: 10000, minTokens: 5000 });
  });

  it("applies text filters on Enter or Filter, not per keystroke", () => {
    renderPage();

    openFilters();
    typeInto("Model", "gpt-4o");
    expect(currentParams().get("model")).toBeNull();
    fireEvent.keyDown(drawerField("Model"), { key: "Enter" });
    expect(currentParams().get("model")).toBe("gpt-4o");
    expect(queryDrawer()).not.toBeInTheDocument();

    openFilters();
    typeInto("Conversation ID", " conv-1 ");
    expect(currentParams().get("conversationId")).toBeNull();
    applyFilters();
    expect(currentParams().get("conversationId")).toBe("conv-1");
  });

  it("caps the text filters at the API's 256 characters", () => {
    renderPage();

    openFilters();
    for (const name of ["Model", "Tool called", "MCP server", "Conversation ID"]) {
      expect(drawerField(name)).toHaveAttribute("maxLength", "256");
    }
  });

  it("removes only the chip's filter, and Clear all removes the rest", () => {
    renderPage("?timeRange=1h&status=error&minSpanCount=20&model=gpt-4o");

    fireEvent.click(screen.getByLabelText("Remove Steps ≥ 20"));
    let params = currentParams();
    expect(params.get("minSpanCount")).toBeNull();
    expect(params.get("status")).toBe("error");
    expect(params.get("model")).toBe("gpt-4o");

    fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
    params = currentParams();
    expect(params.get("status")).toBeNull();
    expect(params.get("model")).toBeNull();
    expect(params.get("timeRange")).toBe("1h");
    expect(lastFilters()).toEqual({});
  });

  it("hides Clear all with a single active filter, status included", () => {
    const { unmount } = renderPage("?status=ok");
    expect(statusButton("Success")).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByRole("button", { name: "Clear all" })).not.toBeInTheDocument();
    unmount();

    renderPage("?model=gpt-4o");
    expect(screen.queryByRole("button", { name: "Clear all" })).not.toBeInTheDocument();
  });

  it("keeps the drawer open when its trace is still in the filtered list", () => {
    renderPage("?selectedTrace=t-err");
    fireEvent.click(statusButton("Errors"));
    expect(currentParams().get("selectedTrace")).toBe("t-err");
  });

  it("closes the drawer when a filter drops its trace", () => {
    renderPage("?selectedTrace=t-ok");
    fireEvent.click(statusButton("Errors"));
    expect(currentParams().get("status")).toBe("error");
    expect(currentParams().get("selectedTrace")).toBeNull();
  });

  it("writes status=error from Errors, and All removes it", () => {
    renderPage("?timeRange=1h");

    expect(statusButton("All")).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(statusButton("Errors"));
    expect(currentParams().get("status")).toBe("error");
    expect(lastFilters()).toEqual({ status: "error" });
    fireEvent.click(statusButton("Success"));
    expect(currentParams().get("status")).toBe("ok");
    fireEvent.click(statusButton("All"));
    expect(currentParams().toString()).toBe("timeRange=1h");
    expect(lastFilters()).toEqual({});
  });

  it("applies drawer picks only on Filter, in one URL update, then closes and shows chips", () => {
    renderPage("?timeRange=1h");

    openFilters();
    expect(filtersButton()).toHaveAttribute("aria-expanded", "true");
    pickPill("Latency at least", "5s");
    typeInto("Model", "gpt-4o");
    expect(currentParams().toString()).toBe("timeRange=1h");
    expect(lastFilters()).toEqual({});

    const updates = searchHistory.length;
    applyFilters();
    expect(searchHistory.length).toBe(updates + 1);
    const params = currentParams();
    expect(params.get("minDurationMs")).toBe("5000");
    expect(params.get("model")).toBe("gpt-4o");
    expect(queryDrawer()).not.toBeInTheDocument();
    expect(filtersButton()).toHaveAttribute("aria-expanded", "false");
    expect(chip("Latency ≥ 5s")).toBeInTheDocument();
    expect(chip("Model: gpt-4o")).toBeInTheDocument();
  });

  it("writes nothing when the applied draft equals the URL's filters", () => {
    renderPage("?timeRange=1h&minDurationMs=5000");

    openFilters();
    const updates = searchHistory.length;
    applyFilters();
    expect(searchHistory.length).toBe(updates);
    expect(queryDrawer()).not.toBeInTheDocument();
  });

  it("discards the draft on the close button and on Escape", () => {
    renderPage("?timeRange=1h");

    openFilters();
    pickPill("Latency at least", "5s");
    fireEvent.click(within(drawer()).getByRole("button", { name: "Close advanced filters" }));
    expect(queryDrawer()).not.toBeInTheDocument();
    expect(currentParams().toString()).toBe("timeRange=1h");
    openFilters();
    expect(pressedPill("Latency at least")).toEqual(["Any"]);

    typeInto("Model", "gpt-4o");
    fireEvent.keyDown(drawerField("Model"), { key: "Escape" });
    expect(queryDrawer()).not.toBeInTheDocument();
    expect(currentParams().toString()).toBe("timeRange=1h");
    openFilters();
    expect(drawerField("Model")).toHaveValue("");
  });

  it("resets the drawer's filters but not status, applying nothing until Filter", () => {
    renderPage("?timeRange=1h&status=error&minDurationMs=5000&model=gpt-4o&toolError=true");

    openFilters();
    fireEvent.click(within(drawer()).getByRole("button", { name: "Reset" }));
    expect(pressedPill("Latency at least")).toEqual(["Any"]);
    expect(drawerField("Model")).toHaveValue("");
    expect(toolFailedBox()).not.toBeChecked();
    expect(currentParams().get("minDurationMs")).toBe("5000");
    expect(currentParams().get("model")).toBe("gpt-4o");

    applyFilters();
    expect(currentParams().toString()).toBe("timeRange=1h&status=error");
    expect(lastFilters()).toEqual({ status: "error" });
  });

  it("opens the drawer on the URL's values from a chip, and its × removes without opening", () => {
    renderPage("?timeRange=1h&minDurationMs=5000&model=gpt-4o");

    fireEvent.click(chip("Latency ≥ 5s")!);
    expect(pressedPill("Latency at least")).toEqual(["5s"]);
    expect(drawerField("Model")).toHaveValue("gpt-4o");
    fireEvent.click(within(drawer()).getByRole("button", { name: "Close advanced filters" }));

    fireEvent.click(screen.getByRole("button", { name: "Remove Model: gpt-4o" }));
    expect(currentParams().get("model")).toBeNull();
    expect(currentParams().get("minDurationMs")).toBe("5000");
    expect(queryDrawer()).not.toBeInTheDocument();
  });

  it("applies the toggle, a chip's × and Clear all straight away while the drawer is open", () => {
    renderPage("?timeRange=1h&minDurationMs=5000&model=gpt-4o");

    openFilters();
    pickPill("Tokens at least", "5k");
    fireEvent.click(statusButton("Errors"));
    expect(currentParams().get("status")).toBe("error");
    fireEvent.click(screen.getByRole("button", { name: "Remove Latency ≥ 5s" }));
    expect(currentParams().get("minDurationMs")).toBeNull();
    expect(pressedPill("Latency at least")).toEqual(["Any"]);
    expect(pressedPill("Tokens at least")).toEqual(["5k"]);
    expect(drawerField("Model")).toHaveValue("gpt-4o");

    fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
    expect(currentParams().toString()).toBe("timeRange=1h");
    expect(pressedPill("Tokens at least")).toEqual(["Any"]);
    expect(drawerField("Model")).toHaveValue("");
    expect(queryDrawer()).toBeInTheDocument();
  });

  it("clears status and every drawer filter with Clear all", () => {
    renderPage(
      "?timeRange=1h&status=error&minDurationMs=2500&minTokens=5000&minSpanCount=20&model=gpt-4o" +
        "&conversationId=conv-1&tool=search_web&toolError=true&mcpServer=github",
    );

    fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
    expect(currentParams().toString()).toBe("timeRange=1h");
    expect(lastFilters()).toEqual({});
    expect(statusButton("All")).toHaveAttribute("aria-pressed", "true");
    expect(screen.queryByRole("button", { name: /^Remove / })).not.toBeInTheDocument();
  });

  it("shows a pasted non-preset latency as its own selected pill", () => {
    renderPage("?minDurationMs=2500");

    expect(chip("Latency ≥ 2500ms")).toBeInTheDocument();
    openFilters();
    expect(pillLabels("Latency at least")).toEqual(["Any", "1s", "2500ms", "5s", "10s", "30s", "Custom"]);
    expect(pressedPill("Latency at least")).toEqual(["2500ms"]);
  });

  it("writes a custom latency value in API units", () => {
    renderPage("?timeRange=1h");

    openFilters();
    pickPill("Latency at least", "Custom");
    expect(drawerField("Custom latency at least")).toHaveValue("");
    typeInto("Custom latency at least", "2.5");
    applyFilters();

    expect(currentParams().get("minDurationMs")).toBe("2500");
    expect(chip("Latency ≥ 2500ms")).toBeInTheDocument();

    openFilters();
    expect(pressedPill("Latency at least")).toEqual(["2500ms"]);
    pickPill("Latency at least", "Custom");
    expect(drawerField("Custom latency at least")).toHaveValue("2.5");
  });

  it("marks an invalid custom value as an error and disables Filter", () => {
    renderPage("?timeRange=1h");

    openFilters();
    pickPill("Latency at least", "Custom");
    for (const bad of ["-1", "5k", "1e3"]) {
      typeInto("Custom latency at least", bad);
      expect(drawerField("Custom latency at least")).toHaveAttribute("aria-invalid", "true");
      expect(applyButton()).toBeDisabled();
    }
    typeInto("Custom latency at least", "2.5");
    expect(drawerField("Custom latency at least")).toHaveAttribute("aria-invalid", "false");
    expect(applyButton()).toBeEnabled();

    typeInto("Custom latency at least", "5k");
    fireEvent.keyDown(drawerField("Custom latency at least"), { key: "Enter" });
    expect(queryDrawer()).toBeInTheDocument();
    expect(currentParams().toString()).toBe("timeRange=1h");
  });

});

describe("tool and MCP server filter URL parsing", () => {
  it("round-trips each new filter through the URL", () => {
    const filters: TraceFilters = { tool: "search_web", toolError: true, mcpServer: "github" };
    const params = withTraceFilters(new URLSearchParams("timeRange=1h"), filters);
    expect(params.toString()).toBe("timeRange=1h&tool=search_web&toolError=true&mcpServer=github");
    expect(parseTraceFilters(params)).toEqual(filters);
  });

  it("drops toolError other than true", () => {
    expect(parseTraceFilters(new URLSearchParams("toolError=yes"))).toEqual({});
    expect(parseTraceFilters(new URLSearchParams("toolError=false"))).toEqual({});
  });

  it("writes toolError only when true", () => {
    const params = withTraceFilters(new URLSearchParams("toolError=true"), { toolError: false });
    expect(params.toString()).toBe("");
  });

  it("labels the new chips, pairing tool with toolError", () => {
    expect(
      traceFilterChips({ tool: "search_web", mcpServer: "github" }).map((c) => c.label),
    ).toEqual(["Tool: search_web", "MCP server: github"]);
    expect(traceFilterChips({ toolError: true }).map((c) => c.label)).toEqual(["Tool failed"]);
    expect(traceFilterChips({ tool: "search_web", toolError: true })).toEqual([
      {
        key: "tool",
        label: "search_web failed",
        name: "Tool",
        value: "search_web failed",
        alsoClears: ["toolError"],
      },
    ]);
  });
});

describe("TracesComponent tool and MCP server filters", () => {
  it("sends today's request from the default page", () => {
    renderPage("?timeRange=1h");

    expect(mockUseTraceList.mock.lastCall).toEqual([
      "ns", "p", "a", "dev", "1h", 10, "desc", undefined, undefined, { filters: {}, paged: true },
    ]);
    expect(currentParams().toString()).toBe("timeRange=1h");
  });

  it("shows the default page's row with no chips and the drawer closed", () => {
    renderPage("?timeRange=1h");

    expect(statusButton("All")).toHaveAttribute("aria-pressed", "true");
    expect(filtersButton()).toHaveAttribute("aria-expanded", "false");
    expect(queryDrawer()).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Remove / })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Clear all" })).not.toBeInTheDocument();
    expect(searchHistory).toEqual(["?timeRange=1h"]);
  });

  it("reproduces a pasted URL's new filters in the request, the drawer and the chips", () => {
    renderPage("?timeRange=1h&tool=search_web&mcpServer=github");

    expect(lastFilters()).toEqual({ tool: "search_web", mcpServer: "github" });
    for (const label of ["Tool: search_web", "MCP server: github"]) {
      expect(chip(label)).toBeInTheDocument();
    }
    openFilters();
    expect(drawerField("Tool called")).toHaveValue("search_web");
    expect(drawerField("MCP server")).toHaveValue("github");
  });

  it("writes Tool, Tool failed and MCP server to the URL", () => {
    renderPage("?timeRange=1h");

    openFilters();
    typeInto("Tool called", " search_web ");
    fireEvent.click(toolFailedBox());
    typeInto("MCP server", "github");
    expect(currentParams().get("tool")).toBeNull();
    applyFilters();

    const params = currentParams();
    expect(params.get("tool")).toBe("search_web");
    expect(params.get("toolError")).toBe("true");
    expect(params.get("mcpServer")).toBe("github");
    expect(lastFilters()).toEqual({ tool: "search_web", toolError: true, mcpServer: "github" });

    openFilters();
    expect(toolFailedBox()).toBeChecked();
    fireEvent.click(toolFailedBox());
    applyFilters();
    expect(currentParams().get("toolError")).toBeNull();
  });

  it("shows tool with toolError as one chip whose × clears both", () => {
    renderPage("?timeRange=1h&status=error&tool=search_web&toolError=true");

    expect(chip("search_web failed")).toBeInTheDocument();
    expect(chip("search_web failed")).toHaveTextContent("Tool search_web failed");
    expect(chip("Tool: search_web")).toBeNull();
    expect(chip("Tool failed")).toBeNull();

    fireEvent.click(screen.getByLabelText("Remove search_web failed"));
    const params = currentParams();
    expect(params.get("tool")).toBeNull();
    expect(params.get("toolError")).toBeNull();
    expect(params.get("status")).toBe("error");
    expect(lastFilters()).toEqual({ status: "error" });
  });

  it("drops toolError other than true from a pasted URL", () => {
    renderPage("?timeRange=1h&toolError=1");

    expect(lastFilters()).toEqual({});
    openFilters();
    expect(toolFailedBox()).not.toBeChecked();
  });

  it("clears the new filters with Clear all", () => {
    renderPage("?timeRange=1h&tool=search_web&toolError=true&mcpServer=github");

    fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
    expect(currentParams().toString()).toBe("timeRange=1h");
    expect(lastFilters()).toEqual({});
  });

  it("never shows the filters drawer over an open trace", () => {
    renderPage("?selectedTrace=t-err");

    openFilters();
    expect(queryDrawer()).not.toBeInTheDocument();
    expect(currentParams().get("selectedTrace")).toBe("t-err");
  });
});

describe("missing conversation filter", () => {
  const noConversationBox = () =>
    within(drawer()).getByRole("checkbox", { name: "Only traces without a conversation ID" });

  it("round-trips missing through the URL and drops it beside a conversation ID", () => {
    const filters: TraceFilters = { missing: ["conversationId"] };
    const params = withTraceFilters(new URLSearchParams("timeRange=1h"), filters);
    expect(params.toString()).toBe("timeRange=1h&missing=conversationId");
    expect(parseTraceFilters(params)).toEqual(filters);
    expect(parseTraceFilters(new URLSearchParams("missing=model"))).toEqual({});
    expect(parseTraceFilters(new URLSearchParams("conversationId=c1&missing=conversationId")))
      .toEqual({ conversationId: "c1" });
  });

  it("labels the chip as no conversation", () => {
    expect(traceFilterChips({ missing: ["conversationId"] }).map((c) => c.label))
      .toEqual(["Conversation: none"]);
  });

  it("writes missing=conversationId from the drawer, disabling and clearing the ID", () => {
    renderPage("?timeRange=1h&conversationId=c1");

    openFilters();
    fireEvent.click(noConversationBox());
    expect(drawerField("Conversation ID")).toBeDisabled();
    applyFilters();

    expect(currentParams().get("missing")).toBe("conversationId");
    expect(currentParams().get("conversationId")).toBeNull();
    expect(lastFilters()).toEqual({ missing: ["conversationId"] });
    expect(chip("Conversation: none")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Remove Conversation: none"));
    expect(currentParams().get("missing")).toBeNull();
    expect(lastFilters()).toEqual({});
  });
});

describe("TracesComponent trace ID search", () => {
  it("opens the trace on Enter, trimmed and lowercased, keeping the other params", () => {
    renderPage("?timeRange=1h&status=error");

    const input = screen.getByRole("textbox", { name: "Go to trace ID" });
    fireEvent.change(input, { target: { value: "  ABC123  " } });
    expect(currentParams().get("selectedTrace")).toBeNull();
    fireEvent.keyDown(input, { key: "Enter" });

    const params = currentParams();
    expect(params.get("selectedTrace")).toBe("abc123");
    expect(params.get("timeRange")).toBe("1h");
    expect(params.get("status")).toBe("error");
  });

  it("opens the trace from the button", () => {
    renderPage();

    fireEvent.change(screen.getByRole("textbox", { name: "Go to trace ID" }), {
      target: { value: "abc123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Go to trace" }));

    expect(currentParams().get("selectedTrace")).toBe("abc123");
  });

  it("does nothing for a blank ID", () => {
    renderPage();

    const input = screen.getByRole("textbox", { name: "Go to trace ID" });
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.click(screen.getByRole("button", { name: "Go to trace" }));

    expect(currentParams().get("selectedTrace")).toBeNull();
  });

  it("closes the filters drawer, discarding its draft, when the search or a row opens a trace", () => {
    const { unmount } = renderPage("?timeRange=1h");

    openFilters();
    pickPill("Latency at least", "5s");
    fireEvent.change(screen.getByRole("textbox", { name: "Go to trace ID" }), {
      target: { value: "abc123" },
    });
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Go to trace ID" }), { key: "Enter" });
    expect(currentParams().get("selectedTrace")).toBe("abc123");
    expect(currentParams().get("minDurationMs")).toBeNull();
    expect(queryDrawer()).not.toBeInTheDocument();
    unmount();

    renderPage("?timeRange=1h");
    openFilters();
    fireEvent.click(screen.getByText("t-ok"));
    expect(currentParams().get("selectedTrace")).toBe("t-ok");
    expect(queryDrawer()).not.toBeInTheDocument();
  });

  it("reads the trace through the spans lookup over the page's window, not the list", () => {
    renderPage();

    fireEvent.change(screen.getByRole("textbox", { name: "Go to trace ID" }), {
      target: { value: "not-in-the-list" },
    });
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Go to trace ID" }), { key: "Enter" });

    expect(vi.mocked(useTrace).mock.lastCall).toEqual([
      "ns",
      "p",
      "a",
      "dev",
      "not-in-the-list",
      "2026-10-01T09:00:00Z",
      "2026-10-01T11:00:00Z",
    ]);
    expect(lastFilters()).toEqual({});
  });
});

describe("trace column URL parsing", () => {
  it("defaults to every column, drops unknown columns, and reads empty as none", () => {
    expect(parseTraceColumns(new URLSearchParams(""))).toEqual([
      "traceId",
      "conversation",
      "input",
      "output",
    ]);
    expect(parseTraceColumns(new URLSearchParams("columns=model,bogus,conversation"))).toEqual([
      "conversation",
    ]);
    expect(parseTraceColumns(new URLSearchParams("columns=conversation,traceId"))).toEqual([
      "traceId",
      "conversation",
    ]);
    expect(parseTraceColumns(new URLSearchParams("columns="))).toEqual([]);
  });
});

describe("TracesComponent columns and cap notice", () => {
  const columnHeader = (name: string) =>
    screen.queryByRole("columnheader", { name, hidden: true });
  const openColumnsMenu = () => fireEvent.click(screen.getByRole("button", { name: "Columns" }));

  it("has no Model column and leaves models to the server's model filter", () => {
    const { unmount } = renderPage("?timeRange=1h");
    expect(columnHeader("Conversation")).toBeInTheDocument();
    expect(columnHeader("Model")).not.toBeInTheDocument();
    expect(lastIncludeModels()).toBeUndefined();
    unmount();

    renderPage("?timeRange=1h&model=gpt-4o");
    expect(columnHeader("Model")).not.toBeInTheDocument();
    expect(lastFilters()).toEqual({ model: "gpt-4o" });
    expect(lastIncludeModels()).toBeUndefined();
    expect(within(screen.getByRole("table")).queryByText("gpt-4o")).not.toBeInTheDocument();
  });

  it("hides Conversation from the menu, which lists no Model item", () => {
    renderPage("?timeRange=1h");

    openColumnsMenu();
    expect(screen.getAllByRole("menuitemcheckbox").map((el) => el.textContent)).toEqual([
      "Trace ID",
      "Conversation",
      "Input",
      "Output",
    ]);
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "Conversation" }));

    expect(currentParams().get("columns")).toBe("traceId,input,output");
    expect(currentParams().get("timeRange")).toBe("1h");
    expect(columnHeader("Conversation")).not.toBeInTheDocument();
  });

  it("shows Trace ID, Conversation, Input and Output by default, with no Name column", () => {
    renderPage("?timeRange=1h");

    const headers = screen.getAllByRole("columnheader").map((el) => el.textContent);
    expect(headers.slice(1, 5)).toEqual(["Trace ID", "Conversation", "Input", "Output"]);
    expect(columnHeader("Name")).not.toBeInTheDocument();
    expect(screen.getByText("t-err")).toBeInTheDocument();
  });

  it("hides Input and Output from the menu", () => {
    renderPage("?timeRange=1h");

    openColumnsMenu();
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "Input" }));
    fireEvent.click(screen.getByRole("menuitemcheckbox", { name: "Output" }));

    expect(currentParams().get("columns")).toBe("traceId,conversation");
    expect(columnHeader("Input")).not.toBeInTheDocument();
    expect(columnHeader("Output")).not.toBeInTheDocument();
  });

  it("copies the trace ID from its cell without opening the trace", async () => {
    renderPage("?timeRange=1h&columns=traceId");

    fireEvent.click(screen.getByRole("button", { name: "Copy trace ID t-ok" }));

    await waitFor(() => expect(copyToClipboard).toHaveBeenCalledWith("t-ok"));
    expect(currentParams().get("selectedTrace")).toBeNull();
  });

  it("reads an older columns link that still lists model", () => {
    renderPage("?columns=conversation,model");

    expect(columnHeader("Conversation")).toBeInTheDocument();
    expect(columnHeader("Model")).not.toBeInTheDocument();
    expect(lastIncludeModels()).toBeUndefined();
  });

  it("sets the conversation filter from the column without opening the trace", () => {
    renderPage("?timeRange=1h");

    fireEvent.click(screen.getByRole("button", { name: "Filter by conversation conv-1" }));

    expect(currentParams().get("conversationId")).toBe("conv-1");
    expect(currentParams().get("timeRange")).toBe("1h");
    expect(currentParams().get("selectedTrace")).toBeNull();
    expect(lastFilters()).toEqual({ conversationId: "conv-1" });
  });

  it("shows the examine-cap banner only when the page was truncated", () => {
    const { unmount } = renderPage();
    expect(screen.queryByText(/traces searched so far/)).not.toBeInTheDocument();
    unmount();

    hookOverrides = { truncated: true, hasMore: true };
    renderPage("?status=error");
    expect(
      screen.getByText(
        "Showing matches from the traces searched so far. Narrow the time range to see more.",
      ),
    ).toBeInTheDocument();
  });

  it("says the list stops here when truncated without a next page", () => {
    hookOverrides = { truncated: true, hasMore: false };
    renderPage("?status=error");

    expect(screen.queryByText(/traces searched so far/)).not.toBeInTheDocument();
    expect(
      screen.getByText(
        "The list stops here: the remaining traces are more than 1,000 traces into this time range. Narrow the time range to see more.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load more traces" })).not.toBeInTheDocument();
  });

  it("shows how far a filtered list searched in the row after the traces", () => {
    hookOverrides = { hasMore: true, lookedBackTo: "2026-10-01T08:14:00Z" };
    const { unmount } = renderPage();
    expect(screen.queryByText(/Searched as far as/)).not.toBeInTheDocument();
    unmount();

    renderPage("?status=error");
    const sentinelRow = screen.getByTestId("traces-sentinel").closest("tr")!;
    expect(
      within(sentinelRow).getByText(/^Searched as far as \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/),
    ).toBeInTheDocument();
  });
});

describe("TracesComponent export warning", () => {
  const LOOKED_BACK_TO = "2026-10-01T08:14:00Z";
  const SPANS_TEXT =
    "Some traces have more than 10,000 spans. The file has the first 10,000 spans of each.";
  const FAILED_3_TEXT =
    "3 traces couldn't be read, so the file may be missing them. Export again to retry.";
  const exportTraces = vi.fn<(params: unknown) => Promise<TraceExportResponse>>();
  const createObjectURL = vi.fn(() => "blob:export");
  let clickSpy: ReturnType<typeof vi.spyOn>;

  const exportResponse = (extra: Partial<TraceExportResponse> = {}): TraceExportResponse => ({
    traces: [],
    totalCount: 0,
    truncated: false,
    spansTruncated: false,
    ...extra,
  });
  const clickExport = () => fireEvent.click(screen.getByRole("button", { name: "Export" }));

  beforeEach(() => {
    exportTraces.mockReset();
    vi.mocked(useExportTraces).mockReturnValue({
      mutateAsync: exportTraces,
      isPending: false,
    } as unknown as ReturnType<typeof useExportTraces>);
    // jsdom has no object URLs and would try to navigate on the link click.
    Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() });
    clickSpy = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  });

  afterEach(() => {
    clickSpy.mockRestore();
    vi.useRealTimers();
  });

  it("says a filtered export stopped early and how far it searched", async () => {
    exportTraces.mockResolvedValue(
      exportResponse({ truncated: true, lookedBackTo: LOOKED_BACK_TO }),
    );
    renderPage("?status=error");

    clickExport();

    expect(
      await screen.findByText(
        "The export stopped before the end of the time range. " +
          `It searched as far as ${formatStartTime(LOOKED_BACK_TO)}. ` +
          "Narrow the time range to export the rest.",
      ),
    ).toBeInTheDocument();
    expect(formatStartTime(LOOKED_BACK_TO)).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
    expect(exportTraces).toHaveBeenCalledWith(expect.objectContaining({ filters: { status: "error" } }));
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(SPANS_TEXT)).not.toBeInTheDocument();
  });

  it("leaves out how far it searched when lookedBackTo is missing", async () => {
    exportTraces.mockResolvedValue(exportResponse({ truncated: true }));
    renderPage("?status=error");

    clickExport();

    expect(
      await screen.findByText(
        "The export stopped before the end of the time range. Narrow the time range to export the rest.",
      ),
    ).toBeInTheDocument();
    expect(createObjectURL).toHaveBeenCalledTimes(1);
  });

  it("says spans were cut, without a search line when no filter is set", async () => {
    exportTraces.mockResolvedValue(exportResponse({ truncated: true, spansTruncated: true }));
    renderPage();

    clickExport();

    expect(await screen.findByText(SPANS_TEXT)).toBeInTheDocument();
    expect(screen.queryByText(/stopped/)).not.toBeInTheDocument();
    expect(createObjectURL).toHaveBeenCalledTimes(1);
  });

  it("says spans were cut and the search may also have stopped early with a filter set", async () => {
    exportTraces.mockResolvedValue(
      exportResponse({ truncated: true, spansTruncated: true, lookedBackTo: LOOKED_BACK_TO }),
    );
    renderPage("?status=error");

    clickExport();

    expect(await screen.findByText(SPANS_TEXT)).toBeInTheDocument();
    expect(
      screen.getByText(
        `The search may also have stopped early: it searched as far as ${formatStartTime(LOOKED_BACK_TO)}.`,
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/export stopped before/)).not.toBeInTheDocument();
    expect(createObjectURL).toHaveBeenCalledTimes(1);
  });

  it("shows no warning when the export is complete", async () => {
    exportTraces.mockResolvedValue(exportResponse({ lookedBackTo: LOOKED_BACK_TO }));
    renderPage("?status=error");

    clickExport();

    await waitFor(() => expect(createObjectURL).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("keeps the warning until it is closed", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    exportTraces.mockResolvedValue(exportResponse({ truncated: true, spansTruncated: true }));
    renderPage();

    clickExport();
    await screen.findByText(SPANS_TEXT);
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    fireEvent.click(document.body);
    expect(screen.getByText(SPANS_TEXT)).toBeInTheDocument();

    fireEvent.click(within(screen.getByRole("alert")).getByRole("button", { name: "Close" }));
    expect(screen.queryByText(SPANS_TEXT)).not.toBeInTheDocument();
  });

  it("clears the warning when a new export starts", async () => {
    let finishSecond!: (resp: TraceExportResponse) => void;
    exportTraces
      .mockResolvedValueOnce(exportResponse({ truncated: true, spansTruncated: true }))
      .mockReturnValueOnce(new Promise((resolve) => (finishSecond = resolve)));
    renderPage();

    clickExport();
    await screen.findByText(SPANS_TEXT);
    clickExport();

    expect(exportTraces).toHaveBeenCalledTimes(2);
    expect(screen.queryByText(SPANS_TEXT)).not.toBeInTheDocument();
    await act(async () => finishSecond(exportResponse()));
    expect(createObjectURL).toHaveBeenCalledTimes(2);
    expect(screen.queryByText(SPANS_TEXT)).not.toBeInTheDocument();
  });

  it("says how many traces couldn't be read, without filters", async () => {
    exportTraces.mockResolvedValue(
      exportResponse({ failedTraceIds: ["trace-1", "trace-2", "trace-3"] }),
    );
    renderPage();

    clickExport();

    expect(await screen.findByText(FAILED_3_TEXT)).toBeInTheDocument();
    expect(createObjectURL).toHaveBeenCalledTimes(1);
  });

  it("says one trace couldn't be read in the singular", async () => {
    exportTraces.mockResolvedValue(exportResponse({ failedTraceIds: ["trace-1"] }));
    renderPage("?status=error");

    clickExport();

    expect(
      await screen.findByText(
        "1 trace couldn't be read, so the file may be missing it. Export again to retry.",
      ),
    ).toBeInTheDocument();
  });

  it("stacks the unread line under the cut-spans line", async () => {
    exportTraces.mockResolvedValue(
      exportResponse({
        truncated: true,
        spansTruncated: true,
        failedTraceIds: ["trace-1", "trace-2", "trace-3"],
      }),
    );
    renderPage();

    clickExport();

    expect(await screen.findByText(SPANS_TEXT)).toBeInTheDocument();
    expect(screen.getByText(FAILED_3_TEXT)).toBeInTheDocument();
  });

  it("stacks the unread line under the stopped-search line", async () => {
    exportTraces.mockResolvedValue(
      exportResponse({
        truncated: true,
        lookedBackTo: LOOKED_BACK_TO,
        failedTraceIds: ["trace-1", "trace-2", "trace-3"],
      }),
    );
    renderPage("?status=error");

    clickExport();

    expect(await screen.findByText(/^The export stopped before the end/)).toBeInTheDocument();
    expect(screen.getByText(FAILED_3_TEXT)).toBeInTheDocument();
  });

  it("shows no warning for an empty unread list", async () => {
    exportTraces.mockResolvedValue(exportResponse({ failedTraceIds: [] }));
    renderPage();

    clickExport();

    await waitFor(() => expect(createObjectURL).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("TracesComponent infinite scroll", () => {
  const loadMoreButton = () => screen.queryByRole("button", { name: "Load more traces" });
  // Lets a resolved loadMore result reach the table.
  const settleLoad = () => act(async () => {});

  it("asks the hook to keep loaded pages through focus and reconnect", () => {
    renderPage();
    expect(mockUseTraceList.mock.lastCall?.[9]?.paged).toBe(true);
  });

  it("loads the next page once when the sentinel comes into view", () => {
    hookOverrides = { hasMore: true };
    renderPage();
    expect(loadMoreButton()).not.toBeInTheDocument();

    scrollToSentinel();
    scrollToSentinel();
    expect(loadMore).toHaveBeenCalledTimes(1);
  });

  it("doesn't load while a page is loading or when there are no more pages", () => {
    hookOverrides = { hasMore: true, isLoadingMore: true };
    const { unmount } = renderPage();
    const sentinelRow = screen.getByTestId("traces-sentinel").closest("tr")!;
    expect(within(sentinelRow).getByRole("progressbar")).toBeInTheDocument();
    scrollToSentinel();
    expect(loadMore).not.toHaveBeenCalled();
    unmount();

    hookOverrides = { hasMore: false };
    renderPage();
    scrollToSentinel();
    expect(loadMore).not.toHaveBeenCalled();
  });

  it("pauses on a page that adds no rows and resumes when a click adds some", async () => {
    hookOverrides = { hasMore: true };
    const { rerenderPage } = renderPage("?status=error");
    loadMore.mockResolvedValueOnce(false);
    scrollToSentinel();
    expect(loadMore).toHaveBeenCalledTimes(1);

    // The page lands with no new matches; the merge still hands over a new array.
    hookOverrides = { hasMore: true, isLoadingMore: true };
    rerenderPage();
    hookOverrides = { hasMore: true, data: { traces: [ALL[0]], totalCount: 1 } };
    rerenderPage();
    await settleLoad();
    expect(loadMoreButton()).toBeInTheDocument();
    scrollToSentinel();
    expect(loadMore).toHaveBeenCalledTimes(1);

    loadMore.mockResolvedValueOnce(true);
    fireEvent.click(loadMoreButton()!);
    expect(loadMore).toHaveBeenCalledTimes(2);

    // The clicked page adds a row, so scrolling loads again.
    const grown = [...ALL, makeTrace("t-new", 1)];
    hookOverrides = { hasMore: true, isLoadingMore: true };
    rerenderPage();
    hookOverrides = { hasMore: true, data: { traces: grown, totalCount: grown.length } };
    rerenderPage();
    await settleLoad();
    expect(loadMoreButton()).not.toBeInTheDocument();
    scrollToSentinel();
    expect(loadMore).toHaveBeenCalledTimes(3);
  });

  it("doesn't pause a list that resets while a page is loading", async () => {
    hookOverrides = { hasMore: true };
    const { rerenderPage } = renderPage("?status=error");
    scrollToSentinel();
    hookOverrides = { hasMore: true, isLoadingMore: true };
    rerenderPage();

    // A new list of the same length lands, and the old load resolves undefined.
    hookOverrides = { hasMore: true, data: { traces: [makeTrace("t-other")], totalCount: 1 } };
    rerenderPage();
    await settleLoad();

    expect(loadMoreButton()).not.toBeInTheDocument();
    scrollToSentinel();
    expect(loadMore).toHaveBeenCalledTimes(2);
  });

  it("resumes auto-loading when a new list replaces a paused one", async () => {
    hookOverrides = { hasMore: true };
    const { rerenderPage } = renderPage("?status=error");
    loadMore.mockResolvedValueOnce(false);
    scrollToSentinel();
    await settleLoad();
    expect(loadMoreButton()).toBeInTheDocument();

    hookOverrides = { hasMore: true, data: { traces: [makeTrace("t-other")], totalCount: 1 } };
    rerenderPage();
    expect(loadMoreButton()).not.toBeInTheDocument();
  });

  it("shows the button under an empty filtered page and doesn't auto-load", () => {
    hookOverrides = { hasMore: true, data: { traces: [], totalCount: 0 } };
    renderPage("?status=error");

    expect(screen.getByText("No traces found!")).toBeInTheDocument();
    scrollToSentinel();
    expect(loadMore).not.toHaveBeenCalled();
    fireEvent.click(loadMoreButton()!);
    expect(loadMore).toHaveBeenCalledTimes(1);
  });

  it("shows Retry after a failed load and stops auto-loading", () => {
    hookOverrides = { hasMore: true, loadError: new Error("upstream down") };
    renderPage();

    expect(screen.getByText("Couldn't load more traces.")).toBeInTheDocument();
    scrollToSentinel();
    expect(loadMore).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(loadMore).toHaveBeenCalledTimes(1);
  });

  it("flips the sort order from the Start Time header", () => {
    renderPage();
    expect(mockUseTraceList.mock.lastCall?.[6]).toBe("desc");

    fireEvent.click(screen.getByText("Start Time"));
    expect(mockUseTraceList.mock.lastCall?.[6]).toBe("asc");
  });

  it.each([
    ["desc", "", "desc"],
    ["asc", "?sortOrder=asc", "asc"],
  ])("puts the sentinel after the last row in %s order", (_, search, sortOrder) => {
    hookOverrides = { hasMore: true };
    renderPage(search);

    expect(mockUseTraceList.mock.lastCall?.[6]).toBe(sortOrder);
    const lastRow = screen.getByText("t-ok").closest("tr")!;
    const sentinel = screen.getByTestId("traces-sentinel");
    const position = lastRow.compareDocumentPosition(sentinel);
    expect(position & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(sentinel.closest("tr")).not.toBe(lastRow);
  });
});

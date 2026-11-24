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

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderHook, type RenderedHook } from "../../test-utils/render-hook";
import type { ConsoleActionPayload } from "../apis/telemetry";

const api = vi.hoisted(() => ({
  reportConsoleActions: vi.fn(),
  reportConsoleActionsOnUnload: vi.fn(),
}));
vi.mock("../apis/telemetry", () => api);

import {
  ConsoleAction,
  isConsoleAnalyticsEnabled,
  markSessionExpired,
  setConsoleAnalyticsEnabled,
  setTelemetrySubject,
  setTelemetryTokenProvider,
  useDialogAnalytics,
  usePageViewTracking,
  useSessionAnalytics,
  useTrack,
  useValidationErrorTracking,
} from "./telemetry";

const tokenProvider = async () => "tok";
const mounted: RenderedHook<unknown>[] = [];

async function mount<T>(useHook: () => T): Promise<RenderedHook<T>> {
  const hook = await renderHook(useHook);
  mounted.push(hook as RenderedHook<unknown>);
  return hook;
}

/** Flushes the shared buffer and returns what was sent. */
async function sent(): Promise<ConsoleActionPayload[]> {
  api.reportConsoleActions.mockClear();
  const hook = await mount(useTrack);
  await act(async () => { await hook.result.current.flush(); });
  return api.reportConsoleActions.mock.calls.flatMap(([batch]) => batch);
}

const names = (actions: ConsoleActionPayload[]) => actions.map((a) => a.action);

beforeEach(async () => {
  // Drain whatever an earlier test left buffered.
  setConsoleAnalyticsEnabled(true);
  setTelemetryTokenProvider(undefined);
  const hook = await renderHook(useTrack);
  await act(async () => { await hook.result.current.flush(); });
  hook.unmount();

  setTelemetryTokenProvider(tokenProvider);
  api.reportConsoleActions.mockResolvedValue({ accepted: 1, dropped: 0 });
  api.reportConsoleActionsOnUnload.mockReturnValue(true);
  window.history.pushState({}, "", "/org/acme-corp/project/checkout/agents");
});

afterEach(() => {
  mounted.splice(0).forEach((hook) => hook.unmount());
  vi.resetAllMocks();
  vi.useRealTimers();
});

describe("analytics switch", () => {
  it("follows the service's runtime config, off unless it says true", () => {
    setConsoleAnalyticsEnabled(undefined);
    expect(isConsoleAnalyticsEnabled()).toBe(false);
    setConsoleAnalyticsEnabled(true);
    expect(isConsoleAnalyticsEnabled()).toBe(true);
  });

  it("buffers nothing while off", async () => {
    setConsoleAnalyticsEnabled(false);
    const hook = await mount(useTrack);
    hook.result.current.track(ConsoleAction.CopySnippet, { snippet: "curl" });
    expect(await sent()).toEqual([]);
  });
});

describe("useTrack", () => {
  it("sends a batch on the flush interval, templated and with the token source", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const hook = await mount(useTrack);
    hook.result.current.track(ConsoleAction.CopySnippet, { snippet: "curl" });
    hook.result.current.track(ConsoleAction.TabSwitch, { tab: "logs" }, "/org/acme-corp/llm-providers/view/router-agent");
    expect(api.reportConsoleActions).not.toHaveBeenCalled();

    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });

    expect(api.reportConsoleActions).toHaveBeenCalledTimes(1);
    const [batch, provider] = api.reportConsoleActions.mock.calls[0];
    expect(provider).toBe(tokenProvider);
    expect(batch).toHaveLength(2);
    expect(batch[0]).toMatchObject({
      action: ConsoleAction.CopySnippet,
      page: "/org/:orgId/project/:projectId/agents",
      dimensions: { snippet: "curl" },
    });
    expect(batch[0].sessionId).toEqual(expect.any(String));
    expect(new Date(batch[0].occurredAt).getTime()).not.toBeNaN();
    expect(batch[1].page).toBe("/org/:orgId/llm-providers/view/:providerId");
    expect(JSON.stringify(batch)).not.toContain("acme-corp");
  });

  it("sends at once when the batch is full", async () => {
    const hook = await mount(useTrack);
    await act(async () => {
      for (let i = 0; i < 50; i += 1) hook.result.current.track(ConsoleAction.Search);
    });
    expect(api.reportConsoleActions).toHaveBeenCalledTimes(1);
    expect(api.reportConsoleActions.mock.calls[0][0]).toHaveLength(50);
  });

  it("drops the buffer when no token source is registered", async () => {
    setTelemetryTokenProvider(undefined);
    const hook = await mount(useTrack);
    hook.result.current.track(ConsoleAction.Search);
    await act(async () => { await hook.result.current.flush(); });
    expect(api.reportConsoleActions).not.toHaveBeenCalled();

    setTelemetryTokenProvider(tokenProvider);
    expect(await sent()).toEqual([]);
  });

  it("swallows a failed send and does not retry it", async () => {
    api.reportConsoleActions.mockRejectedValue(new Error("collector down"));
    const hook = await mount(useTrack);
    hook.result.current.track(ConsoleAction.Search);
    await act(async () => { await expect(hook.result.current.flush()).resolves.toBeUndefined(); });

    api.reportConsoleActions.mockResolvedValue({ accepted: 0, dropped: 0 });
    expect(await sent()).toEqual([]);
  });

  it("keeps actions reported during a send for the next batch", async () => {
    let release: () => void = () => undefined;
    api.reportConsoleActions.mockImplementationOnce(
      () => new Promise((resolve) => { release = () => resolve({ accepted: 1, dropped: 0 }); }),
    );
    const hook = await mount(useTrack);
    hook.result.current.track(ConsoleAction.Search);
    const inFlight = hook.result.current.flush();
    hook.result.current.track(ConsoleAction.FilterApplied);
    release();
    await act(async () => { await inFlight; });

    expect(names(api.reportConsoleActions.mock.calls[0][0])).toEqual([ConsoleAction.Search]);
    expect(names(await sent())).toEqual([ConsoleAction.FilterApplied]);
  });
});

describe("unload flush", () => {
  it("hands the buffer to a keepalive send when the page hides", async () => {
    const hook = await mount(useTrack);
    hook.result.current.track(ConsoleAction.DownloadArtifact);

    await act(async () => { window.dispatchEvent(new Event("pagehide")); });

    expect(api.reportConsoleActionsOnUnload).toHaveBeenCalledTimes(1);
    const [batch, token] = api.reportConsoleActionsOnUnload.mock.calls[0];
    expect(names(batch)).toEqual([ConsoleAction.DownloadArtifact]);
    expect(token).toBe("tok");
    expect(await sent()).toEqual([]);
  });

  it("flushes when the tab is hidden, but not when it becomes visible", async () => {
    const hook = await mount(useTrack);
    const setVisibility = (state: string) => {
      Object.defineProperty(document, "visibilityState", { configurable: true, value: state });
      document.dispatchEvent(new Event("visibilitychange"));
    };
    hook.result.current.track(ConsoleAction.Search);

    await act(async () => setVisibility("visible"));
    expect(api.reportConsoleActionsOnUnload).not.toHaveBeenCalled();

    await act(async () => setVisibility("hidden"));
    expect(api.reportConsoleActionsOnUnload).toHaveBeenCalledTimes(1);
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
  });

  it("sends nothing on unload with an empty buffer, and survives a failing token", async () => {
    await mount(useTrack);
    await act(async () => { window.dispatchEvent(new Event("pagehide")); });
    expect(api.reportConsoleActionsOnUnload).not.toHaveBeenCalled();

    setTelemetryTokenProvider(() => Promise.reject(new Error("no token")));
    const hook = await mount(useTrack);
    hook.result.current.track(ConsoleAction.Search);
    await act(async () => { window.dispatchEvent(new Event("pagehide")); });
    expect(api.reportConsoleActionsOnUnload).not.toHaveBeenCalled();
  });
});

describe("usePageViewTracking", () => {
  it("reports each distinct route once, with where the user came from", async () => {
    let path = "/org/acme-corp/llm-providers/view/router-agent";
    const hook = await mount(() => usePageViewTracking(path, "org"));
    await hook.rerender();
    path = "/org/acme-corp/llm-providers/view/router-agent?tab=2";
    await hook.rerender();
    path = "/org/acme-corp/project/checkout";
    await hook.rerender();

    const views = await sent();
    expect(views.map((v) => v.dimensions)).toEqual([
      { route: "/org/:orgId/llm-providers/view/:providerId", entity_scope: "org" },
      {
        route: "/org/:orgId/project/:projectId",
        entity_scope: "org",
        referrer_route: "/org/:orgId/llm-providers/view/:providerId",
      },
    ]);
    expect(views.every((v) => v.action === ConsoleAction.PageView)).toBe(true);
    expect(views[1].page).toBe("/org/:orgId/project/:projectId");
  });

  it("omits the scope when none is given", async () => {
    await mount(() => usePageViewTracking("/org/acme-corp"));
    expect((await sent())[0].dimensions).toEqual({ route: "/org/:orgId" });
  });
});

describe("useDialogAnalytics", () => {
  it("reports an open and, without completion, an abandonment with time on form", async () => {
    const now = vi.spyOn(Date, "now").mockReturnValue(1_000_000);
    let open = false;
    let step = "basics";
    const hook = await mount(() => useDialogAnalytics("agent", open, { trigger: "header", step }));
    open = true;
    await hook.rerender();
    step = "review";
    now.mockReturnValue(1_042_400);
    await hook.rerender();
    open = false;
    await hook.rerender();
    now.mockRestore();

    expect((await sent()).map((a) => [a.action, a.dimensions])).toEqual([
      [ConsoleAction.DialogOpened, { entity: "agent", trigger: "header" }],
      [ConsoleAction.FormAbandoned, { entity: "agent", furthest_step: "review", seconds_on_form: 42 }],
    ]);
  });

  it("reports no abandonment after a completed submission", async () => {
    let open = true;
    const hook = await mount(() => useDialogAnalytics("project", open));
    act(() => hook.result.current.markCompleted());
    open = false;
    await hook.rerender();

    const actions = await sent();
    expect(names(actions)).toEqual([ConsoleAction.DialogOpened]);
    expect(actions[0].dimensions).toEqual({ entity: "project", trigger: "unspecified" });
  });

  it("defaults the step when none was given", async () => {
    let open = true;
    const hook = await mount(() => useDialogAnalytics("project", open));
    open = false;
    await hook.rerender();
    const abandoned = (await sent())[1];
    expect(abandoned.dimensions).toMatchObject({ furthest_step: "unspecified" });
  });
});

describe("useValidationErrorTracking", () => {
  it("reports the field, never a value", async () => {
    const hook = await mount(() => useValidationErrorTracking("agent"));
    hook.result.current("displayName");
    expect((await sent()).map((a) => [a.action, a.dimensions])).toEqual([
      [ConsoleAction.ValidationError, { entity: "agent", field: "displayName" }],
    ]);
  });
});

describe("useSessionAnalytics", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("reports the start, and a first session only for a new browser", async () => {
    const first = await mount(() => useSessionAnalytics());
    await first.rerender();
    let actions = await sent();
    expect(names(actions)).toEqual([ConsoleAction.SessionStart, ConsoleAction.FirstSession]);
    expect(actions[0].dimensions).toMatchObject({
      referrer: "direct",
      viewport: `${window.innerWidth}x${window.innerHeight}`,
    });

    first.unmount();
    await mount(() => useSessionAnalytics());
    actions = await sent();
    expect(names(actions)).toEqual([ConsoleAction.SessionStart]);
  });

  it("reports a parked session expiry once the owner signs back in", async () => {
    setTelemetrySubject("user-a");
    markSessionExpired("/org/acme-corp/project/checkout");
    const who: { subject?: string } = {};
    const hook = await mount(() => useSessionAnalytics(who.subject));
    who.subject = "user-a";
    await hook.rerender();
    await hook.rerender();

    const expired = (await sent()).filter((a) => a.action === ConsoleAction.SessionExpired);
    expect(expired.map((a) => a.dimensions)).toEqual([{ page: "/org/:orgId/project/:projectId" }]);
    setTelemetrySubject(undefined);
  });

  it("reports uncaught errors by name only", async () => {
    await mount(() => useSessionAnalytics());
    await sent();

    await act(async () => {
      const error = new ErrorEvent("error", { error: new TypeError("secret data"), cancelable: true });
      error.preventDefault();
      window.dispatchEvent(error);
      window.dispatchEvent(Object.assign(new Event("unhandledrejection"), {
        reason: Object.assign(new Error("x"), { name: "AbortError" }),
      }));
      window.dispatchEvent(Object.assign(new Event("unhandledrejection"), { reason: undefined }));
    });

    const errors = await sent();
    expect(errors.map((a) => a.dimensions)).toEqual([
      { component: "window", error_name: "TypeError" },
      { component: "promise", error_name: "AbortError" },
      { component: "promise", error_name: "UnhandledRejection" },
    ]);
    expect(JSON.stringify(errors)).not.toContain("secret data");
  });

  it("ends the session once on page hide and starts a new one on a cache restore", async () => {
    await mount(() => useSessionAnalytics());
    await sent();

    await act(async () => {
      window.dispatchEvent(new Event("pagehide"));
      window.dispatchEvent(new Event("pagehide"));
    });
    const ended = api.reportConsoleActionsOnUnload.mock.calls.flatMap(([batch]) => batch);
    const ends = ended.filter((a: ConsoleActionPayload) => a.action === ConsoleAction.SessionEnd);
    expect(ends).toHaveLength(1);
    expect(ends[0].dimensions).toEqual({
      duration_seconds: expect.any(Number),
      page_count: expect.any(Number),
      action_count: expect.any(Number),
    });
    const endedSession = ends[0].sessionId;

    await act(async () => {
      window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: false }));
    });
    expect(await sent()).toEqual([]);

    await act(async () => {
      window.dispatchEvent(Object.assign(new Event("pageshow"), { persisted: true }));
    });
    const restarted = await sent();
    expect(names(restarted)).toEqual([ConsoleAction.SessionStart]);
    expect(restarted[0].sessionId).not.toBe(endedSession);
  });
});

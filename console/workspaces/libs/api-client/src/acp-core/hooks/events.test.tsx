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
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  invalidatedKeys, renderHook, seed, testQueryClient, waitFor, type RenderedHook,
} from "../../test-utils/render-hook";

const ui = vi.hoisted(() => ({ pushSnackBar: vi.fn(), getToken: async () => "tok" }));
const api = vi.hoisted(() => {
  const names = [
    "createEventDestination", "createEventSubscription", "deleteEventDestination",
    "deleteEventSubscription", "getEventDestination", "listAvailableEventDestinations",
    "listDestinationKinds", "listEventDeliveries", "listEventDestinations",
    "listEventSubscriptions", "listEventTypes", "rotateEventDestinationSecret",
    "testEventDestination", "updateEventDestination", "updateEventSubscription",
  ] as const;
  return Object.fromEntries(names.map((n) => [n, vi.fn()])) as Record<
    (typeof names)[number],
    ReturnType<typeof vi.fn>
  >;
});
const getRuntimeConfig = vi.hoisted(() => vi.fn());

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: ui.pushSnackBar }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: ui.getToken, isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/events", () => api);
vi.mock("../apis/runtime-config", () => ({ getRuntimeConfig }));

import * as hooks from "./events";

type Api = keyof typeof api;
let rendered: RenderedHook<unknown> | undefined;
async function render<T>(useHook: () => T) {
  const hook = await renderHook(useHook);
  rendered = hook as RenderedHook<unknown>;
  return hook;
}

afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  vi.resetAllMocks();
});

const orgT = { scope: "org", orgName: "o" } as const;
const projectT = { scope: "project", orgName: "o", projName: "p" } as const;
const agentT = { scope: "agent", orgName: "o", projName: "p", agentName: "a" } as const;
const query = { limit: 20 } as never;

describe("useEventsEnabled", () => {
  it.each([
    [{ eventsEnabled: true }, true],
    [{ eventsEnabled: false }, false],
    [{}, false],
  ])("reads %j from the runtime config as %s", async (config, enabled) => {
    getRuntimeConfig.mockResolvedValue(config);
    const hook = await render(() => ({
      enabled: hooks.useEventsEnabled(),
    }));
    await waitFor(() => hook.client.getQueryData(["runtime-config"]) !== undefined);
    await hook.rerender();
    expect(hook.result.current.enabled).toBe(enabled);
  });

  it("is off when the config cannot be read", async () => {
    getRuntimeConfig.mockRejectedValue(new Error("down"));
    const hook = await render(() => ({ enabled: hooks.useEventsEnabled() }));
    await waitFor(() => hook.client.getQueryState(["runtime-config"])?.status === "error");
    expect(hook.result.current.enabled).toBe(false);
  });
});

type QueryRow = [string, () => { data?: unknown }, Api, unknown[], unknown[]];
const queries: QueryRow[] = [
  ["useEventTypes", () => hooks.useEventTypes("o", "project" as never), "listEventTypes",
    ["o", "project"], ["event-types", "o", "project"]],
  ["useDestinationKinds", () => hooks.useDestinationKinds("o"), "listDestinationKinds", ["o"],
    ["event-destination-kinds", "o"]],
  ["useListEventDestinations", () => hooks.useListEventDestinations("o"), "listEventDestinations",
    ["o"], ["event-destinations", "o"]],
  ["useAvailableEventDestinations (project)", () => hooks.useAvailableEventDestinations(projectT),
    "listAvailableEventDestinations", [projectT], ["event-destinations", "o", "available", projectT]],
  ["useAvailableEventDestinations (agent)", () => hooks.useAvailableEventDestinations(agentT),
    "listAvailableEventDestinations", [agentT], ["event-destinations", "o", "available", agentT]],
  ["useEventDestination", () => hooks.useEventDestination("o", "d1"), "getEventDestination",
    ["o", "d1"], ["event-destinations", "o", "one", "d1"]],
  ["useListEventSubscriptions (org)", () => hooks.useListEventSubscriptions(orgT),
    "listEventSubscriptions", [orgT], ["event-subscriptions", orgT]],
  ["useListEventDeliveries", () => hooks.useListEventDeliveries(agentT, query),
    "listEventDeliveries", [agentT, query], ["event-deliveries", agentT, query]],
];

describe.each(queries)("%s", (_name, useHook, fn, args, key) => {
  it("loads through the API with the caller's token and caches under its key", async () => {
    api[fn].mockResolvedValue({ items: ["x"] });
    const hook = await render(useHook);
    await waitFor(() => hook.result.current.data !== undefined);
    expect(hook.result.current.data).toEqual({ items: ["x"] });
    expect(api[fn]).toHaveBeenCalledWith(...args, ui.getToken);
    expect(hook.client.getQueryData(key)).toEqual({ items: ["x"] });
  });
});

describe("query gating", () => {
  it.each([
    ["event types without an org", () => hooks.useEventTypes(undefined, "org" as never), "listEventTypes"],
    ["destination kinds without an org", () => hooks.useDestinationKinds(undefined), "listDestinationKinds"],
    ["destinations when switched off", () => hooks.useListEventDestinations("o", false), "listEventDestinations"],
    ["destinations without an org", () => hooks.useListEventDestinations(undefined), "listEventDestinations"],
    ["available destinations at org scope", () => hooks.useAvailableEventDestinations(orgT), "listAvailableEventDestinations"],
    ["available destinations for an agent target missing its agent",
      () => hooks.useAvailableEventDestinations({ ...agentT, agentName: undefined } as never),
      "listAvailableEventDestinations"],
    ["available destinations when switched off",
      () => hooks.useAvailableEventDestinations(projectT, false), "listAvailableEventDestinations"],
    ["one destination without an id", () => hooks.useEventDestination("o", undefined), "getEventDestination"],
    ["subscriptions without an org",
      () => hooks.useListEventSubscriptions({ scope: "org" } as never), "listEventSubscriptions"],
    ["subscriptions for a project target missing its project",
      () => hooks.useListEventSubscriptions({ scope: "project", orgName: "o" } as never),
      "listEventSubscriptions"],
    ["deliveries when switched off", () => hooks.useListEventDeliveries(orgT, query, false), "listEventDeliveries"],
  ] as [string, () => { fetchStatus: string }, Api][])("%s stays idle", async (_name, useHook, fn) => {
    const hook = await render(useHook);
    expect(hook.result.current.fetchStatus).toBe("idle");
    expect(api[fn]).not.toHaveBeenCalled();
  });
});

const SEEDED = [
  ["event-destinations", "o"],
  ["event-destinations", "o", "one", "d1"],
  ["event-subscriptions", orgT],
  ["event-types", "o", "org"],
  ["event-deliveries", orgT, query],
] as const;
const EVENTS_STATE = [
  ["event-destinations", "o"],
  ["event-destinations", "o", "one", "d1"],
  ["event-subscriptions", orgT],
];

const body = { name: "hook" };
type MutationRow = [
  string, () => { mutateAsync: (v: never) => Promise<unknown> }, unknown, Api, unknown[],
  string | undefined, unknown[][],
];
const mutations: MutationRow[] = [
  ["useCreateEventDestination", () => hooks.useCreateEventDestination("o"), body,
    "createEventDestination", ["o", body], "Destination created successfully", EVENTS_STATE],
  ["useUpdateEventDestination", () => hooks.useUpdateEventDestination("o"), { id: "d1", body },
    "updateEventDestination", ["o", "d1", body], "Destination updated successfully", EVENTS_STATE],
  ["useDeleteEventDestination", () => hooks.useDeleteEventDestination("o"), "d1",
    "deleteEventDestination", ["o", "d1"], "Destination deleted successfully", EVENTS_STATE],
  ["useRotateEventDestinationSecret", () => hooks.useRotateEventDestinationSecret("o"), "d1",
    "rotateEventDestinationSecret", ["o", "d1"], "Signing Secret updated successfully", []],
  ["useTestEventDestination", () => hooks.useTestEventDestination("o"), "d1",
    "testEventDestination", ["o", "d1"], undefined, []],
  ["useCreateEventSubscription", () => hooks.useCreateEventSubscription(projectT), body,
    "createEventSubscription", [projectT, body], "Subscription created successfully", EVENTS_STATE],
  ["useUpdateEventSubscription", () => hooks.useUpdateEventSubscription(projectT), { id: "s1", body },
    "updateEventSubscription", [projectT, "s1", body], "Subscription updated successfully",
    EVENTS_STATE],
  ["useDeleteEventSubscription", () => hooks.useDeleteEventSubscription(agentT), "s1",
    "deleteEventSubscription", [agentT, "s1"], "Subscription deleted successfully", EVENTS_STATE],
];

describe.each(mutations)("%s", (_name, useHook, variables, fn, args, message, invalidated) => {
  it("calls the API, reports the outcome and refreshes exactly the affected queries", async () => {
    api[fn].mockResolvedValue({ delivered: false });
    const client = testQueryClient();
    seed(client, SEEDED);
    const hook = await renderHook(useHook, { client });
    rendered = hook as RenderedHook<unknown>;

    let result: unknown;
    await act(async () => { result = await hook.result.current.mutateAsync(variables as never); });

    expect(result).toEqual({ delivered: false });
    expect(api[fn]).toHaveBeenCalledWith(...args, ui.getToken);
    if (message) {
      expect(ui.pushSnackBar).toHaveBeenCalledWith({ message, type: "success" });
    } else {
      // A test send shows its outcome from the result, not a snackbar.
      expect(ui.pushSnackBar).not.toHaveBeenCalled();
    }
    expect(invalidatedKeys(client, SEEDED)).toEqual(invalidated);
  });

  it("shows an error and refreshes nothing when the API fails", async () => {
    api[fn].mockRejectedValue(Object.assign(new Error("HTTP error! status: 500"), { status: 500 }));
    const client = testQueryClient();
    seed(client, SEEDED);
    const hook = await renderHook(useHook, { client });
    rendered = hook as RenderedHook<unknown>;

    await act(async () => {
      await hook.result.current.mutateAsync(variables as never).catch(() => undefined);
    });

    expect(ui.pushSnackBar).toHaveBeenCalledWith(expect.objectContaining({ type: "error" }));
    expect(invalidatedKeys(client, SEEDED)).toEqual([]);
  });
});

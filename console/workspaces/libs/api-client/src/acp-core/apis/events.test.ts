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

import { afterEach, describe, expect, it, vi } from "vitest";
import type * as Utils from "../../shared/utils";

const http = vi.hoisted(() => ({
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));
vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof Utils>()),
  ...http,
}));

import * as api from "./events";

const getToken = async () => "tok";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const ORG = "/api/v1/orgs/my%20org/events";
const PROJECT = "/api/v1/orgs/my%20org/projects/p%2F1/events";
const AGENT = "/api/v1/orgs/my%20org/projects/p%2F1/agents/a%201/events";
const orgTarget = { scope: "org", orgName: "my org" } as const;
const projectTarget = { scope: "project", orgName: "my org", projName: "p/1" } as const;
const agentTarget = { scope: "agent", orgName: "my org", projName: "p/1", agentName: "a 1" } as const;
const body = { name: "hook" } as never;

afterEach(() => vi.resetAllMocks());

describe("eventsBase", () => {
  it("builds the events path of an org, a project and an agent", () => {
    expect(api.eventsBase(orgTarget)).toBe(ORG);
    expect(api.eventsBase(projectTarget)).toBe(PROJECT);
    expect(api.eventsBase(agentTarget)).toBe(AGENT);
    expect(api.eventsBase({ scope: "org" } as never)).toBe("/api/v1/orgs/default/events");
  });

  it("refuses a project or agent target that is missing its names", () => {
    expect(() => api.eventsBase({ scope: "project", orgName: "o" } as never)).toThrow(
      "projName is required",
    );
    expect(() => api.eventsBase({ scope: "agent", orgName: "o", projName: "p" } as never)).toThrow(
      "agentName is required",
    );
  });
});

type Method = keyof typeof http;
type Row = [string, Method, () => Promise<unknown>, string, Record<string, unknown>, unknown?];
const calls: Row[] = [
  ["listEventTypes", "httpGET", () => api.listEventTypes("my org", "project" as never, getToken),
    `${ORG}/event-types`, { searchParams: { scope: "project" } }],
  ["listDestinationKinds", "httpGET", () => api.listDestinationKinds("my org", getToken),
    `${ORG}/destination-kinds`, {}],
  ["listEventDestinations", "httpGET", () => api.listEventDestinations("my org", getToken),
    `${ORG}/destinations`, {}],
  ["listAvailableEventDestinations", "httpGET",
    () => api.listAvailableEventDestinations(agentTarget, getToken), `${AGENT}/destinations`, {}],
  ["getEventDestination", "httpGET", () => api.getEventDestination("my org", "d/1", getToken),
    `${ORG}/destinations/d%2F1`, {}],
  ["createEventDestination", "httpPOST", () => api.createEventDestination("my org", body, getToken),
    `${ORG}/destinations`, {}, body],
  ["updateEventDestination", "httpPUT",
    () => api.updateEventDestination("my org", "d/1", body, getToken),
    `${ORG}/destinations/d%2F1`, {}, body],
  ["rotateEventDestinationSecret", "httpPOST",
    () => api.rotateEventDestinationSecret("my org", "d/1", getToken),
    `${ORG}/destinations/d%2F1/rotate-secret`, {}, {}],
  ["testEventDestination", "httpPOST", () => api.testEventDestination("my org", "d/1", getToken),
    `${ORG}/destinations/d%2F1/test`, {}, {}],
  ["listEventSubscriptions", "httpGET", () => api.listEventSubscriptions(projectTarget, getToken),
    `${PROJECT}/subscriptions`, {}],
  ["createEventSubscription", "httpPOST",
    () => api.createEventSubscription(projectTarget, body, getToken),
    `${PROJECT}/subscriptions`, {}, body],
  ["updateEventSubscription", "httpPUT",
    () => api.updateEventSubscription(orgTarget, "s 1", body, getToken),
    `${ORG}/subscriptions/s%201`, {}, body],
];

describe.each(calls)("%s", (_name, method, call, path, options, sentBody) => {
  it("sends the request with encoded ids and the token, and returns the body", async () => {
    http[method].mockResolvedValue(json({ id: "1" }));

    await expect(call()).resolves.toEqual({ id: "1" });

    const args = http[method].mock.calls[0];
    expect(args[0]).toBe(path);
    if (sentBody !== undefined) expect(args[1]).toEqual(sentBody);
    expect(args[args.length - 1]).toEqual({ token: "tok", ...options });
  });

  it("lets the helper's HTTP error through", async () => {
    http[method].mockRejectedValue(Object.assign(new Error("Forbidden"), { status: 403 }));
    await expect(call()).rejects.toMatchObject({ status: 403 });
  });
});

describe("deletes", () => {
  it("deletes a destination and a subscription, resolving to nothing", async () => {
    http.httpDELETE.mockResolvedValue(new Response(null, { status: 204 }));
    await expect(api.deleteEventDestination(undefined, "d/1", getToken)).resolves.toBeUndefined();
    await expect(api.deleteEventSubscription(agentTarget, "s/1")).resolves.toBeUndefined();
    expect(http.httpDELETE.mock.calls).toEqual([
      ["/api/v1/orgs/default/events/destinations/d%2F1", { token: "tok" }],
      [`${AGENT}/subscriptions/s%2F1`, { token: undefined }],
    ]);
  });

  it("lets a refusal through", async () => {
    http.httpDELETE.mockRejectedValue(Object.assign(new Error("In use"), { status: 409 }));
    await expect(api.deleteEventDestination("o", "d")).rejects.toThrow("In use");
  });
});

describe("listEventDeliveries", () => {
  it("sends only the filters that are set", async () => {
    http.httpGET.mockImplementation(async () => json({ deliveries: [] }));

    await api.listEventDeliveries(projectTarget, {
      destinationId: "d1", subscriptionId: "s1", status: "failed", limit: 25, offset: 50,
    } as never, getToken);
    await api.listEventDeliveries(projectTarget, { offset: 0 } as never);

    expect(http.httpGET.mock.calls).toEqual([
      [`${PROJECT}/deliveries`, {
        searchParams: { destinationId: "d1", subscriptionId: "s1", status: "failed", limit: "25", offset: "50" },
        token: "tok",
      }],
      [`${PROJECT}/deliveries`, { searchParams: {}, token: undefined }],
    ]);
  });
});

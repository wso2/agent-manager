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

/**
 * Route normalization is the one piece of console telemetry whose failure is a
 * privacy incident rather than a bug: it is what keeps customer-chosen org,
 * project, agent and environment handles out of an external analytics service.
 *
 * It has already regressed once. The first implementation keyed on `orgs` and
 * `projects` while the app's real routes are `/org/:orgId` and
 * `project/:projectId` — singular — so it matched nothing and forwarded the
 * handles verbatim. These tests exist so that particular mistake, and the
 * class it belongs to, cannot come back silently.
 */

import { beforeEach, describe, expect, it } from "vitest";
import {
  markSessionExpired,
  normalizeRoute,
  sanitizePage,
  setTelemetrySubject,
  takePendingSessionExpiry,
} from "./telemetry";

/** Anything a customer could have named. If one of these survives, we leaked. */
const CUSTOMER_NAMES = [
  "acme-corp",
  "checkout-service",
  "router-agent",
  "prod-eu-west",
  "billing-admins",
];

function leaksAnyName(route: string): boolean {
  return CUSTOMER_NAMES.some((name) => route.includes(name));
}

describe("sanitizePage", () => {
  it.each([
    ["", ""],
    ["/org/acme-corp", "/org/acme-corp"],
    ["/logs?search=my-secret-query", "/logs"],
    ["/traces#span-42", "/traces"],
    ["/logs?search=x#frag", "/logs"],
  ])("strips query and fragment: %s -> %s", (input, expected) => {
    expect(sanitizePage(input)).toBe(expected);
  });
});

describe("normalizeRoute", () => {
  // The exact shapes the console produces, taken from the real route map.
  it.each([
    ["/org/acme-corp", "/org/:orgId"],
    [
      "/org/acme-corp/project/checkout-service/agents/router-agent",
      "/org/:orgId/project/:projectId/agents/:agentId",
    ],
    ["/org/acme-corp/llm-providers/view/router-agent", "/org/:orgId/llm-providers/view/:providerId"],
    [
      "/org/acme-corp/settings/identities/roles/billing-admins",
      "/org/:orgId/settings/identities/roles/:roleId",
    ],
  ])("templates %s", (input, expected) => {
    expect(normalizeRoute(input)).toBe(expected);
  });

  // A static segment must beat a parameter at the same depth. Sorting by string
  // length put `:roleId` (7 chars) ahead of `create` (6), so the create page
  // was reported as the role-detail page and the two merged into one metric.
  it("prefers a static segment over a parameter at the same depth", () => {
    expect(normalizeRoute("/org/acme-corp/thunder-instances/roles/create")).toBe(
      "/org/:orgId/thunder-instances/roles/create",
    );
    expect(normalizeRoute("/org/acme-corp/thunder-instances/roles/billing-admins")).toBe(
      "/org/:orgId/thunder-instances/roles/:roleId",
    );
  });

  // The regression that shipped: singular `/org/`, not `/orgs/`.
  it("masks the org handle on the singular /org/ route", () => {
    const route = normalizeRoute("/org/acme-corp");
    expect(route).not.toContain("acme-corp");
  });

  it("masks every customer-chosen segment across a deep route", () => {
    const route = normalizeRoute(
      "/org/acme-corp/project/checkout-service/agents/router-agent/environment/prod-eu-west",
    );
    expect(leaksAnyName(route)).toBe(false);
  });

  // The whitelist fallback is what makes a route added later safe by default.
  it("masks unknown segments rather than passing them through", () => {
    const route = normalizeRoute("/org/acme-corp/some-unmapped-page/billing-admins");
    expect(leaksAnyName(route)).toBe(false);
    expect(route).toContain(":id");
  });

  it("strips the query string before templating", () => {
    const route = normalizeRoute("/org/acme-corp/logs?search=checkout-service");
    expect(leaksAnyName(route)).toBe(false);
  });

  it.each([[""], ["/"], ["//"]])("handles the degenerate path %s", (input) => {
    expect(() => normalizeRoute(input)).not.toThrow();
    expect(normalizeRoute(input).startsWith("/")).toBe(true);
  });
});

describe("session-expiry record", () => {
  const KEY = "amp.console.session-expired";

  beforeEach(() => {
    window.localStorage.clear();
    setTelemetrySubject(undefined);
  });

  it("records nothing when no user is known", () => {
    markSessionExpired("/org/acme-corp/logs");
    expect(window.localStorage.getItem(KEY)).toBeNull();
  });

  it("records the owner and a normalized route", () => {
    setTelemetrySubject("user-a");
    markSessionExpired("/org/acme-corp/project/checkout-service");
    const stored = JSON.parse(window.localStorage.getItem(KEY) ?? "{}");
    expect(stored.sub).toBe("user-a");
    expect(leaksAnyName(stored.route)).toBe(false);
  });

  it("reports the record to the user who owns it, once", () => {
    setTelemetrySubject("user-a");
    markSessionExpired("/org/acme-corp");
    expect(takePendingSessionExpiry("user-a")).toEqual({ route: "/org/:orgId" });
    expect(takePendingSessionExpiry("user-a")).toBeUndefined();
  });

  // The reported bug: user B signing in on the same browser must not report
  // user A's expiry under B's identity — and must not consume it either, so A
  // can still report it on their own next sign-in.
  it("does not report another user's expiry, and leaves it for them", () => {
    setTelemetrySubject("user-a");
    markSessionExpired("/org/acme-corp");

    expect(takePendingSessionExpiry("user-b")).toBeUndefined();
    expect(window.localStorage.getItem(KEY)).not.toBeNull();
    expect(takePendingSessionExpiry("user-a")).toEqual({ route: "/org/:orgId" });
  });

  it("drops a record older than the reporting window", () => {
    window.localStorage.setItem(
      KEY,
      JSON.stringify({ route: "/org/:orgId", at: Date.now() - 48 * 3600 * 1000, sub: "user-a" }),
    );
    expect(takePendingSessionExpiry("user-a")).toBeUndefined();
    expect(window.localStorage.getItem(KEY)).toBeNull();
  });

  it("drops a malformed record instead of throwing", () => {
    window.localStorage.setItem(KEY, "{not json");
    expect(() => takePendingSessionExpiry("user-a")).not.toThrow();
    expect(window.localStorage.getItem(KEY)).toBeNull();
  });
});

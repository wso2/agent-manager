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

import { describe, expect, it, vi } from "vitest";

vi.mock("@agent-management-platform/views", () => ({ useSnackBar: () => ({ pushSnackBar: vi.fn() }) }));
vi.mock("@agent-management-platform/auth", () => ({ useAuthHooks: () => ({}) }));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));

import { legacyOrgRedirectPath, orgRouteId } from "./organizations";

const NS = "wc-01a0c318-7a4ef447";
const cloudOrgs = [{ name: NS, handle: "agent-manager-team" }];

describe("orgRouteId", () => {
  it("uses the handle when the backend reports one", () => {
    expect(orgRouteId({ name: "wc-01a0c318-7a4ef447", handle: "agent-manager-team" }))
      .toBe("agent-manager-team");
  });

  it("falls back to the namespace-backed name without a handle", () => {
    expect(orgRouteId({ name: "wc-01a0c318-7a4ef447" })).toBe("wc-01a0c318-7a4ef447");
    expect(orgRouteId({ name: "default", handle: "" })).toBe("default");
  });

  it("is undefined while there is no org yet", () => {
    expect(orgRouteId(undefined)).toBeUndefined();
  });
});

describe("legacyOrgRedirectPath", () => {
  it("moves an old namespace link onto the handle, keeping the rest of the path", () => {
    expect(legacyOrgRedirectPath(`/org/${NS}/catalog`, NS, cloudOrgs))
      .toBe("/org/agent-manager-team/catalog");
    expect(legacyOrgRedirectPath(`/org/${NS}/project/default/agents/a1/traces`, NS, cloudOrgs))
      .toBe("/org/agent-manager-team/project/default/agents/a1/traces");
    expect(legacyOrgRedirectPath(`/org/${NS}`, NS, cloudOrgs)).toBe("/org/agent-manager-team");
  });

  it("leaves a handle link alone", () => {
    expect(legacyOrgRedirectPath("/org/agent-manager-team/catalog", "agent-manager-team", cloudOrgs))
      .toBeUndefined();
  });

  it("leaves links alone where the name is the route id", () => {
    // Local setup: handle and namespace are both "default".
    expect(legacyOrgRedirectPath("/org/default/catalog", "default", [{ name: "default", handle: "default" }]))
      .toBeUndefined();
    // No handle reported (e.g. several orgs): the name stays in the URL.
    expect(legacyOrgRedirectPath("/org/default", "default", [{ name: "default" }, { name: "other" }]))
      .toBeUndefined();
  });

  it("does not redirect an org the user can't see, or before orgs load", () => {
    expect(legacyOrgRedirectPath("/org/someone-else/catalog", "someone-else", cloudOrgs)).toBeUndefined();
    expect(legacyOrgRedirectPath(`/org/${NS}/catalog`, NS, undefined)).toBeUndefined();
    expect(legacyOrgRedirectPath("/", undefined, cloudOrgs)).toBeUndefined();
  });

  it("only rewrites the org segment of the path", () => {
    // A project that happens to share the namespace's name keeps its name.
    expect(legacyOrgRedirectPath(`/org/${NS}/project/${NS}`, NS, cloudOrgs))
      .toBe(`/org/agent-manager-team/project/${NS}`);
  });
});

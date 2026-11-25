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

import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({
  tokenScopes: { enforced: true, resolved: true, scopes: new Set<string>() },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useTokenScopes: () => hoisted.tokenScopes,
}));

import { useEventsAccess } from "./useEventsAccess";

const withScopes = (...scopes: string[]) => {
  hoisted.tokenScopes = { enforced: true, resolved: true, scopes: new Set(scopes) };
};

beforeEach(() => withScopes());

describe("useEventsAccess", () => {
  it("allows everything when scopes are not enforced", () => {
    hoisted.tokenScopes = { enforced: false, resolved: false, scopes: new Set() };

    const { result } = renderHook(() => useEventsAccess("org"));

    expect(result.current).toEqual({
      resolved: true,
      canReadOrg: true,
      canManageDestinations: true,
      canManageSubscriptions: true,
    });
  });

  it("lets events:manage read and manage the organization's events", () => {
    withScopes("amp:events:manage");

    const { result } = renderHook(() => useEventsAccess("org"));

    expect(result.current).toEqual({
      resolved: true,
      canReadOrg: true,
      canManageDestinations: true,
      canManageSubscriptions: true,
    });
  });

  it("lets events:read only read the organization's events", () => {
    withScopes("amp:events:read");

    const { result } = renderHook(() => useEventsAccess("org"));

    expect(result.current.canReadOrg).toBe(true);
    expect(result.current.canManageDestinations).toBe(false);
    expect(result.current.canManageSubscriptions).toBe(false);
  });

  it("ties project and agent subscriptions to their update scope", () => {
    withScopes("amp:project:update");
    expect(renderHook(() => useEventsAccess("project")).result.current.canManageSubscriptions).toBe(true);
    expect(renderHook(() => useEventsAccess("agent")).result.current.canManageSubscriptions).toBe(false);

    withScopes("amp:agent:update");
    const agent = renderHook(() => useEventsAccess("agent")).result.current;
    expect(agent.canManageSubscriptions).toBe(true);
    expect(agent.canReadOrg).toBe(false);
  });

  it("reports unresolved scopes", () => {
    hoisted.tokenScopes = { enforced: true, resolved: false, scopes: new Set() };

    expect(renderHook(() => useEventsAccess("org")).result.current.resolved).toBe(false);
  });
});

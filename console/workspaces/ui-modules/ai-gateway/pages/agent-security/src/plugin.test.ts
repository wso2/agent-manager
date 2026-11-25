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

import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const routeAgent = vi.hoisted(() => vi.fn());

vi.mock("@agent-management-platform/api-client", () => ({
  useRouteAgent: routeAgent,
}));

vi.mock("./SecurityPage", () => ({ SecurityPage: () => null }));

import { agentSecurityPlugin } from "./plugin";

const navItem = agentSecurityPlugin.contributions.find((c) => c.type === "nav-item");
const useActivation = (navItem as { useActivation: () => boolean | undefined }).useActivation;

const activationFor = (result: { data?: unknown; isLoading: boolean }) => {
  routeAgent.mockReturnValue(result);
  return renderHook(() => useActivation()).result.current;
};

beforeEach(() => routeAgent.mockReset());

describe("Credentials menu entry", () => {
  it("waits while the agent loads", () => {
    expect(activationFor({ isLoading: true })).toBeUndefined();
  });

  it("is hidden when there is no agent", () => {
    expect(activationFor({ data: undefined, isLoading: false })).toBe(false);
  });

  it("is shown for external agents and agent-api agents only", () => {
    expect(
      activationFor({ data: { provisioning: { type: "external" } }, isLoading: false }),
    ).toBe(true);
    expect(
      activationFor({
        data: { provisioning: { type: "internal" }, agentType: { type: "agent-api" } },
        isLoading: false,
      }),
    ).toBe(true);
    expect(
      activationFor({
        data: { provisioning: { type: "internal" }, agentType: { type: "chat-api" } },
        isLoading: false,
      }),
    ).toBe(false);
  });

  it("routes to the credentials page", async () => {
    const route = agentSecurityPlugin.contributions.find((c) => c.type === "route") as {
      path: string;
      load: () => Promise<{ default: unknown }>;
    };
    expect(route.path).toMatch(/security/);
    const { SecurityPage } = await import("./SecurityPage");
    await expect(route.load()).resolves.toEqual({ default: SecurityPage });
  });
});

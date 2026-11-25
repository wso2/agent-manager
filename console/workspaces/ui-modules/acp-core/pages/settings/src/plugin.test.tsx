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
  profileManagement: true,
  access: new Set<string>(),
  eventsEnabled: true,
  tokenScopes: { enforced: true, scopes: new Set<string>() },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useEventsEnabled: () => hoisted.eventsEnabled,
  useTokenScopes: () => hoisted.tokenScopes,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureFlag: () => hoisted.profileManagement,
  useFeatureManagement: () => ({ hasAccess: (k: string) => hoisted.access.has(k) }),
}));
vi.mock("./index", () => ({
  metaData: { levels: { organization: "SettingsOrganization" } },
  OrgEvents: "OrgEvents",
  ProjectEvents: "ProjectEvents",
  AgentEvents: "AgentEvents",
}));

import { settingsPlugin } from "./plugin";

const contribution = (id: string) => {
  const c = settingsPlugin.contributions.find((x) => x.id === id);
  if (!c) throw new Error(`no contribution ${id}`);
  return c as unknown as {
    path?: string;
    load?: () => Promise<{ default: unknown }>;
    useActivation?: () => boolean;
  };
};
const activated = (id: string) =>
  renderHook(() => contribution(id).useActivation!()).result.current;

beforeEach(() => {
  hoisted.profileManagement = true;
  hoisted.access = new Set(["member_read"]);
  hoisted.eventsEnabled = true;
  hoisted.tokenScopes = { enforced: true, scopes: new Set() };
});

describe("settingsPlugin", () => {
  it("mounts each page on its route", async () => {
    expect(contribution("settings-page").path).toBe("/org/:orgId/settings/*");
    expect(contribution("org-events-page").path).toBe("/org/:orgId/events/*");
    expect((await contribution("settings-page").load!()).default).toBe("SettingsOrganization");
    expect((await contribution("org-events-page").load!()).default).toBe("OrgEvents");
    expect((await contribution("project-events-page").load!()).default).toBe("ProjectEvents");
    expect((await contribution("agent-events-page").load!()).default).toBe("AgentEvents");
  });

  it("shows Settings with profile management and some identity access", () => {
    expect(activated("settings-nav")).toBe(true);

    hoisted.access = new Set();
    expect(activated("settings-nav")).toBe(false);

    hoisted.access = new Set(["group_read"]);
    hoisted.profileManagement = false;
    expect(activated("settings-nav")).toBe(false);
  });

  it("shows org Events only with the events scopes", () => {
    expect(activated("org-events-nav")).toBe(false);

    hoisted.tokenScopes = { enforced: true, scopes: new Set(["amp:events:read"]) };
    expect(activated("org-events-nav")).toBe(true);

    hoisted.tokenScopes = { enforced: false, scopes: new Set() };
    expect(activated("org-events-nav")).toBe(true);

    hoisted.eventsEnabled = false;
    expect(activated("org-events-nav")).toBe(false);
  });

  it("shows project and agent Events whenever events are enabled", () => {
    expect(activated("project-events-nav")).toBe(true);
    expect(activated("agent-events-nav")).toBe(true);

    hoisted.eventsEnabled = false;
    expect(activated("project-events-nav")).toBe(false);
  });
});

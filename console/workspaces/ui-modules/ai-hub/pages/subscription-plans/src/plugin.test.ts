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

import { describe, expect, it, vi } from "vitest";
import { PLUGIN_API_VERSION, absoluteRouteMap } from "@agent-management-platform/types";
import { subscriptionPlansPlugin } from "./plugin";
import IndexDefault, { metaData, PlanConditionChips, SubscriptionPlansOrganization } from "./index";

const access = vi.hoisted(() => ({ keys: [] as string[] }));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureAccess: (key: string) => {
    access.keys.push(key);
    return key === "subscription_plan_read";
  },
}));
vi.mock("./SubscriptionPlans.Organization", () => ({
  SubscriptionPlansOrganization: () => null,
}));
vi.mock("./subComponents/PlanConditionChips", () => ({
  PlanConditionChips: () => null,
}));

describe("subscriptionPlansPlugin", () => {
  it("declares the plugin at the current API version", () => {
    expect(subscriptionPlansPlugin.id).toBe("subscription-plans");
    expect(subscriptionPlansPlugin.name).toBe("Subscription Plans");
    expect(subscriptionPlansPlugin.apiVersion).toBe(PLUGIN_API_VERSION);
  });

  it("mounts the org page under the subscription plans path", async () => {
    const route = subscriptionPlansPlugin.contributions.find((c) => c.type === "route");
    expect(route).toMatchObject({
      id: "org-page",
      level: "org",
      path: `${absoluteRouteMap.children.org.children.subscriptionPlans.path}/*`,
    });
    const loaded = await (route as { load: () => Promise<{ default: unknown }> }).load();
    expect(loaded.default).toBe(SubscriptionPlansOrganization);
  });

  it("shows the nav item to users who can read plans", () => {
    const nav = subscriptionPlansPlugin.contributions.find((c) => c.type === "nav-item") as {
      section: string;
      label: string;
      route: unknown;
      useActivation: () => boolean;
    };
    expect(nav).toMatchObject({ label: "Subscription Plans", section: "manage" });
    expect(nav.route).toBe(absoluteRouteMap.children.org.children.subscriptionPlans);
    expect(nav.useActivation()).toBe(true);
    expect(access.keys).toContain("subscription_plan_read");
  });
});

describe("index", () => {
  it("exports the org page as the default and in its metadata", () => {
    expect(IndexDefault).toBe(SubscriptionPlansOrganization);
    expect(metaData.component).toBe(SubscriptionPlansOrganization);
    expect(metaData.levels?.organization).toBe(SubscriptionPlansOrganization);
    expect(metaData.path).toBe("/subscription-plans");
    expect(PlanConditionChips).toBeDefined();
  });
});

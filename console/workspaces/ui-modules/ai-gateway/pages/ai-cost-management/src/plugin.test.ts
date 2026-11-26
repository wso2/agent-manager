/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({ access: new Set<string>() }));

vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureManagement: () => ({
    hasAccess: (feature: string) => hoisted.access.has(feature),
  }),
}));

import { aiCostManagementPlugin } from "./plugin";

// Loading the page component imports the whole page, which is slow on a busy runner.
vi.setConfig({ testTimeout: 60_000 });

const contributions = aiCostManagementPlugin.contributions as Array<
  Record<string, any> // eslint-disable-line @typescript-eslint/no-explicit-any
>;
const routes = contributions.filter((c) => c.type === "route");
const navItems = contributions.filter((c) => c.type === "nav-item");

describe("aiCostManagementPlugin", () => {
  it("identifies the plugin", () => {
    expect(aiCostManagementPlugin.id).toBe("ai-cost-management");
    expect(aiCostManagementPlugin.name).toBe("Cost management");
  });

  it("activates only with the cost_read feature", () => {
    const activation = aiCostManagementPlugin.useActivation as () => boolean;
    hoisted.access.clear();
    expect(activation()).toBe(false);
    hoisted.access.add("cost_read");
    expect(activation()).toBe(true);
    hoisted.access.clear();
  });

  it("registers one wildcard page route per level", () => {
    expect(routes.map((r) => [r.id, r.level])).toEqual([
      ["org-page", "org"],
      ["project-page", "project"],
      ["agent-page", "agent"],
    ]);
    routes.forEach((r) => expect(r.path.endsWith("/*")).toBe(true));
    expect(new Set(routes.map((r) => r.path)).size).toBe(3);
  });

  it("lazy-loads the page component", async () => {
    const loaded = await routes[0].load();
    expect(loaded.default).toBeTypeOf("function");
  });

  it("adds Dashboard and Budgets nav items for every level", () => {
    for (const level of ["org", "project", "agent"]) {
      const items = navItems.filter((n) => n.level === level);
      const byId = Object.fromEntries(items.map((n) => [n.id, n]));
      expect(byId[`${level}-overview-nav`].label).toBe("Dashboard");
      expect(byId[`${level}-overview-nav`].order).toBe(10);
      expect(byId[`${level}-budgets-nav`].label).toBe("Budgets");
      expect(byId[`${level}-budgets-nav`].order).toBe(20);
      items.forEach((n) => {
        expect(n.section).toBe("cost-management");
        expect(n.useActivation).toBeTypeOf("function");
      });
    }
  });

  it("limits the overview nav item to its own path", () => {
    const overview = navItems.find((n) => n.id === "org-overview-nav")!;
    expect(overview.route.path).toBe(overview.route.wildPath);
  });

  it("offers the Pricing catalog on the organization level only", () => {
    const pricing = navItems.filter((n) => n.label === "Pricing catalog");
    expect(pricing).toHaveLength(1);
    expect(pricing[0].id).toBe("org-pricing-nav");
    expect(pricing[0].level).toBe("org");
    expect(pricing[0].order).toBe(30);
    expect(navItems).toHaveLength(7);
  });

  it("gives nav items the same activation rule as the plugin", () => {
    hoisted.access.add("cost_read");
    navItems.forEach((n) => expect(n.useActivation()).toBe(true));
    hoisted.access.clear();
    navItems.forEach((n) => expect(n.useActivation()).toBe(false));
  });
});

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

import { describe, expect, it } from "vitest";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { configureAgentPlugin } from "./plugin";
import * as pages from "./index";

type Loadable = {
  id: string;
  type: string;
  path?: string;
  load?: () => Promise<{ default: unknown }>;
};

const contributions = configureAgentPlugin.contributions as unknown as Loadable[];
const byId = (id: string) => contributions.find((c) => c.id === id) as Loadable;
const configure =
  absoluteRouteMap.children.org.children.projects.children.agents.children.configure;

describe("configureAgentPlugin", () => {
  it.each([
    ["page", configure.path, pages.ConfigureComponent],
    ["add-llm-provider-page", configure.children.llmProviders.children.add.path, pages.AddLLMProviderComponent],
    ["edit-llm-provider-page", configure.children.llmProviders.children.edit.path, pages.AddLLMProviderComponent],
    ["view-llm-provider-page", configure.children.llmProviders.children.view.path, pages.ViewLLMProviderComponent],
    ["view-mcp-server-page", configure.children.mcpProxies.children.view.path, pages.ViewMCPServerComponent],
  ])("routes %s to its page", async (id, path, component) => {
    const route = byId(id);
    expect(route.type).toBe("route");
    expect(route.path).toBe(path);
    await expect(route.load?.()).resolves.toEqual({ default: component });
  });

  it.each([
    ["overview-agent-card", pages.EnvAgentCardSummary],
    ["overview-configs", pages.EnvConfigsSection],
  ])("mounts the %s panel on the agent overview", async (id, component) => {
    const panel = byId(id);
    expect(panel.type).toBe("panel");
    await expect(panel.load?.()).resolves.toEqual({ default: component });
  });

  it("adds a Configure nav item", () => {
    expect(byId("nav")).toMatchObject({ type: "nav-item", label: "Configure", route: configure });
  });
});

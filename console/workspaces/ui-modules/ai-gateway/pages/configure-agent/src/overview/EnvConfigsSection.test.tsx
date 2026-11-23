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

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FeatureManagementProvider } from "@agent-management-platform/feature-management";

type Item = { uuid: string; name: string; type: string };

const state = vi.hoisted(() => ({
  llmList: { data: undefined as unknown, isError: false },
  mcpList: { data: undefined as unknown, isError: false },
  llmListCalls: [] as unknown[][],
  llmConfigs: {} as Record<string, { data?: unknown; isLoading: boolean; isError: boolean }>,
  mcpConfigs: {} as Record<string, { data?: unknown; isLoading: boolean; isError: boolean }>,
  proxy: { data: undefined as unknown, isLoading: false, isError: false },
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListAgentModelConfigs: (...args: unknown[]) => {
    state.llmListCalls.push(args);
    return state.llmList;
  },
  useListAgentMCPConfigs: () => state.mcpList,
  useGetAgentModelConfig: ({ configId }: { configId: string }) =>
    state.llmConfigs[configId] ?? { isLoading: true, isError: false },
  useLLMPoliciesCatalog: () => ({
    data: { data: [{ name: "pii", displayName: "PII Masking" }] },
    isLoading: false,
  }),
  useListCatalogLLMProviders: () => ({
    data: { entries: [{ handle: "openai-prov", template: "openai" }] },
  }),
  useListLLMProviderTemplates: () => ({
    data: { templates: [{ id: "openai", name: "OpenAI", metadata: { logoUrl: "/openai.svg" } }] },
  }),
  useGetAgentMCPConfig: ({ configId }: { configId: string }) =>
    state.mcpConfigs[configId] ?? { isLoading: true, isError: false },
  useListEnvironments: () => ({
    data: [{ name: "dev", id: "env-dev" }],
    isLoading: false,
    isError: false,
  }),
  useGetMCPProxy: () => state.proxy,
}));

import { EnvConfigsSection } from "./EnvConfigsSection";

const list = (items: Item[]) => ({
  data: { configs: items, pagination: { count: items.length } },
  isError: false,
});

const llmConfig = (envs: Record<string, unknown>) => ({
  data: { envMappings: envs },
  isLoading: false,
  isError: false,
});

const renderSection = (canWrite = true) =>
  render(
    <FeatureManagementProvider featureAccess={{ agent_write: canWrite }}>
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <EnvConfigsSection orgId="o" projectId="p" agentId="a" envId="dev" />
      </MemoryRouter>
    </FeatureManagementProvider>,
  );

beforeEach(() => {
  state.llmList = list([]);
  state.mcpList = list([]);
  state.llmListCalls = [];
  state.llmConfigs = {};
  state.mcpConfigs = {};
  state.proxy = { data: undefined, isLoading: false, isError: false };
});

describe("EnvConfigsSection", () => {
  it("renders nothing when the agent has no configurations", () => {
    const { container } = renderSection();
    expect(container).toBeEmptyDOMElement();
  });

  it("previews only the LLM configurations deployed to this environment", () => {
    state.llmList = list([
      { uuid: "l1", name: "chat", type: "llm" },
      { uuid: "l2", name: "prod-only", type: "llm" },
    ]);
    state.llmConfigs = {
      l1: llmConfig({
        dev: {
          configuration: {
            providerName: "openai-prov",
            providerPolicies: [{ name: "pii" }],
            policies: [{ name: "pii" }, { name: "toxicity" }],
          },
        },
      }),
      l2: llmConfig({ prod: { configuration: { providerName: "openai-prov" } } }),
    };
    renderSection();
    expect(screen.getByText("LLM Providers")).toBeInTheDocument();
    expect(screen.getByText("chat")).toBeInTheDocument();
    expect(screen.getByText("OpenAI")).toBeInTheDocument();
    // Provider and agent guardrails, deduplicated by display name.
    expect(screen.getByText("Guardrails: PII Masking, toxicity")).toBeInTheDocument();
    expect(screen.queryByText("prod-only")).not.toBeInTheDocument();
    expect(screen.getByText("chat").closest("a")).toHaveAttribute(
      "href",
      "/org/o/project/p/agents/a/configure/llm-providers/view/l1",
    );
    expect(screen.getByRole("link", { name: "Configure LLM" })).toBeInTheDocument();
  });

  it("caps the preview and counts the rest", () => {
    const items = ["a", "b", "c", "d", "e"].map((n) => ({ uuid: n, name: `cfg-${n}`, type: "llm" }));
    state.llmList = list(items);
    for (const item of items) {
      state.llmConfigs[item.uuid] = llmConfig({ dev: { configuration: {} } });
    }
    renderSection();
    expect(screen.getByText("cfg-c")).toBeInTheDocument();
    expect(screen.queryByText("cfg-d")).not.toBeInTheDocument();
    expect(screen.getByText("+2 more")).toBeInTheDocument();
    expect(screen.getAllByText("No guardrails configured")).toHaveLength(3);
  });

  it("fetches every configuration when the first page is not all of them", () => {
    state.llmList = { data: { configs: [], pagination: { count: 25 } }, isError: false };
    renderSection();
    expect(state.llmListCalls).toContainEqual([
      { orgName: "o", projName: "p", agentName: "a" },
      { limit: 25, offset: 0 },
    ]);
  });

  it("reports a configuration whose details failed to load", () => {
    state.llmList = list([{ uuid: "l1", name: "chat", type: "llm" }]);
    state.llmConfigs = { l1: { isLoading: false, isError: true } };
    renderSection();
    expect(screen.getByText("Unable to load llm providers. Try again later.")).toBeInTheDocument();
  });

  it("reports a failed MCP list", () => {
    state.mcpList = { data: undefined, isError: true };
    renderSection();
    expect(screen.getByText("Unable to load mcp servers. Try again later.")).toBeInTheDocument();
  });

  it("lists the tools the environment's MCP endpoint exposes", () => {
    state.mcpList = list([{ uuid: "m1", name: "weather", type: "mcp" }]);
    state.mcpConfigs = {
      m1: llmConfig({ dev: { configuration: { proxyName: "weather-proxy" } } }),
    };
    state.proxy = {
      data: {
        name: "Weather Proxy",
        endpoints: [
          {
            environments: [{ environmentUuid: "env-dev" }],
            capabilities: { tools: [{ name: "forecast" }, { name: "forecast" }, { name: " " }, { name: "alerts" }] },
          },
        ],
      },
      isLoading: false,
      isError: false,
    };
    renderSection(false);
    expect(screen.getByText("MCP Servers")).toBeInTheDocument();
    expect(screen.getByText("Weather Proxy")).toBeInTheDocument();
    expect(screen.getByText("Tools: forecast, alerts")).toBeInTheDocument();
    // No add tile without write access.
    expect(screen.queryByRole("link", { name: "Configure MCP" })).not.toBeInTheDocument();
  });

  it("says when the MCP proxy's tools could not be loaded", () => {
    state.mcpList = list([{ uuid: "m1", name: "weather", type: "mcp" }]);
    state.mcpConfigs = { m1: llmConfig({ dev: { configuration: { proxyId: "weather-proxy" } } }) };
    state.proxy = { data: undefined, isLoading: false, isError: true };
    renderSection();
    expect(screen.getByText("Unable to load tools")).toBeInTheDocument();
  });
});

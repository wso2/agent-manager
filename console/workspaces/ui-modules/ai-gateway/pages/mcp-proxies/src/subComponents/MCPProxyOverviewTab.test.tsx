/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { MCPEndpointConfig, MCPProxy } from "@agent-management-platform/types";
import { MCPProxyOverviewTab } from "./MCPProxyOverviewTab";

vi.setConfig({ testTimeout: 30000 });

const proxy = { id: "github", name: "GitHub", version: "1.0.0" } as MCPProxy;

const config = {
  id: "primary",
  security: { enabled: true, apiKey: { enabled: true, key: "X-API-Key", in: "header" } },
  capabilities: {
    tools: [
      { name: "search", description: "Search repositories" },
      { name: "delete_repo" },
      { name: "  " },
    ],
    resources: [{ uri: "file:///readme" }],
    prompts: [],
  },
  policies: [
    {
      name: "mcp-acl-list",
      version: "v1",
      params: { tools: { mode: "allow", exceptions: ["delete_repo"] } },
    },
  ],
} as unknown as MCPEndpointConfig;

describe("MCPProxyOverviewTab", () => {
  it("summarises auth, tool access and deployment", () => {
    render(
      <MCPProxyOverviewTab
        proxy={proxy}
        config={config}
        envChips={[
          { id: "env-dev", label: "Development", status: "Deployed" },
          { id: "env-prod", label: "Production" },
        ]}
      />,
    );

    expect(screen.getByText("API Key")).toBeInTheDocument();
    // The unnamed tool counts as neither allowed nor disabled.
    expect(screen.getByText("2 allowed · 1 disabled")).toBeInTheDocument();
    expect(screen.getByText("Development · Deployed")).toBeInTheDocument();
    expect(screen.getByText("Production")).toBeInTheDocument();
  });

  it("marks blocked tools as denied in the capability list", () => {
    render(<MCPProxyOverviewTab proxy={proxy} config={config} />);

    expect(screen.getByText("Total: 3")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Tools"));
    expect(screen.getByText("Allowed")).toBeInTheDocument();
    expect(screen.getByText("Denied")).toBeInTheDocument();
    // An unnamed capability gets a positional name.
    expect(screen.getByText("Tool 3")).toBeInTheDocument();

    fireEvent.click(screen.getByText("search"));
    expect(screen.getByText("Search repositories")).toBeInTheDocument();
  });

  it("shows a resource's URI and empty sections", () => {
    render(<MCPProxyOverviewTab proxy={proxy} config={config} />);

    fireEvent.click(screen.getByText("Resources"));
    fireEvent.click(screen.getAllByText("file:///readme")[0]);
    expect(screen.getAllByText("file:///readme")).toHaveLength(2);

    fireEvent.click(screen.getByText("Prompts"));
    expect(screen.getByText("No prompts found.")).toBeInTheDocument();
  });

  it("reports no tools and no environments", () => {
    render(
      <MCPProxyOverviewTab
        proxy={proxy}
        config={{ id: "primary", capabilities: {} } as unknown as MCPEndpointConfig}
      />,
    );

    expect(screen.getByText("No tools")).toBeInTheDocument();
    expect(screen.getByText("No environments")).toBeInTheDocument();
    expect(screen.getByText("None")).toBeInTheDocument();
  });

  it("renders nothing without a proxy and only placeholders while loading", () => {
    const { container, rerender } = render(
      <MCPProxyOverviewTab proxy={undefined} config={config} />,
    );
    expect(container).toBeEmptyDOMElement();

    rerender(<MCPProxyOverviewTab proxy={proxy} config={config} isLoading />);
    expect(screen.queryByText("Auth Type")).not.toBeInTheDocument();
  });
});

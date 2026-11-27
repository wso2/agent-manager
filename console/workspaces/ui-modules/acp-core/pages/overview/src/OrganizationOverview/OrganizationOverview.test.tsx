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

import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ResourceSummariesProvider, type ResourceSummaryOption } from "@agent-management-platform/views";
import type { ResourceSummaryState } from "@agent-management-platform/types";

const hoisted = vi.hoisted(() => ({
  givenName: "Ada" as string | undefined,
  canWriteProject: true,
  projects: { data: undefined as unknown, isLoading: false },
}));

vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({
    userInfo: hoisted.givenName ? { givenName: hoisted.givenName } : undefined,
  }),
}));
vi.mock("@agent-management-platform/api-client", () => ({
  useListProjects: () => hoisted.projects,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canWrite: hoisted.canWriteProject }),
}));

import { OrganizationOverview } from "./OrganizationOverview";

const project = (name: string) => ({
  metadata: { name, creationTimestamp: "2026-01-01T00:00:00Z" },
  spec: { displayName: `${name} project`, description: `${name} description` },
});

const summary = (title: string, state: ResourceSummaryState): ResourceSummaryOption =>
  ({
    key: `mod:${title}`,
    pluginId: "mod",
    id: title,
    type: "resource-summary",
    title,
    description: `${title} description`,
    icon: () => <svg />,
    useSummary: () => state,
  }) as unknown as ResourceSummaryOption;

const renderPage = (summaries: ResourceSummaryOption[] = []) =>
  render(
    <MemoryRouter initialEntries={["/org/acme"]} future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <ResourceSummariesProvider value={summaries}>
        <Routes>
          <Route path="/org/:orgId" element={<OrganizationOverview />} />
        </Routes>
      </ResourceSummariesProvider>
    </MemoryRouter>,
  );

beforeEach(() => {
  hoisted.givenName = "Ada";
  hoisted.canWriteProject = true;
  hoisted.projects = { data: { list: [], pagination: { total: 0 } }, isLoading: false };
});

describe("OrganizationOverview", () => {
  it("greets the signed-in user by their given name", () => {
    renderPage();

    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Hello, Ada");
  });

  it("says just Hello without a given name", () => {
    hoisted.givenName = undefined;
    renderPage();

    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent(/^Hello$/);
  });

  it("lists the first three projects, each linking to its overview, and links to the rest", () => {
    const projects = ["alpha", "beta", "gamma", "delta"].map(project);
    hoisted.projects = { data: { list: projects, pagination: { total: 4 } }, isLoading: false };
    renderPage();

    expect(screen.getByRole("link", { name: /alpha project/ })).toHaveAttribute("href", "/org/acme/project/alpha");
    expect(screen.getByText("gamma project")).toBeInTheDocument();
    expect(screen.queryByText("delta project")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /More projects/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create project" })).toHaveAttribute("href", "/org/acme/newProject");
  });

  it("invites a user who can write projects to create the first one", () => {
    renderPage();

    expect(screen.getByText("Create a project to group your agents and their configuration.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Create project" })).toBeInTheDocument();
  });

  it("only says there are no projects to a user who cannot create one", () => {
    hoisted.canWriteProject = false;
    renderPage();

    expect(screen.getByText("No projects yet.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Create project" })).not.toBeInTheDocument();
  });

  it("shows neither projects nor the empty message while projects load", () => {
    hoisted.projects = { data: undefined, isLoading: true };
    renderPage();

    expect(screen.queryByText("No projects yet.")).not.toBeInTheDocument();
    expect(screen.queryByText(/Create a project to group/)).not.toBeInTheDocument();
  });

  it("shows the resources that have something and leaves out the empty ones", async () => {
    renderPage([
      summary("LLM providers", { count: 2, actionLabel: "View providers", to: "/org/acme/llm-providers" }),
      summary("MCP servers", { count: 0, actionLabel: "Add a server", to: "/org/acme/mcp" }),
    ]);

    expect(await screen.findByText("LLM providers")).toBeInTheDocument();
    expect(screen.queryByText("MCP servers")).not.toBeInTheDocument();
  });

  it("shows every resource while the organization has none of them yet", async () => {
    renderPage([
      summary("LLM providers", { count: 0, actionLabel: "Add a provider", to: "/org/acme/llm-providers" }),
      summary("MCP servers", { count: 0, actionLabel: "Add a server", to: "/org/acme/mcp" }),
    ]);

    expect(await screen.findByText("LLM providers")).toBeInTheDocument();
    expect(screen.getByText("MCP servers")).toBeInTheDocument();
  });
});

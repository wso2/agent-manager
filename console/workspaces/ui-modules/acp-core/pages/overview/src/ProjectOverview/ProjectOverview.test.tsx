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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";

const hoisted = vi.hoisted(() => ({
  canWriteProject: true,
  canCreateAgent: true,
  project: { data: undefined as unknown, isLoading: false },
  agents: { data: undefined as unknown, isSuccess: false, isFetching: false },
  panelProps: [] as unknown[],
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetProject: () => hoisted.project,
  useListAgents: () => hoisted.agents,
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canWrite: hoisted.canWriteProject }),
  useFeatureAccess: () => hoisted.canCreateAgent,
}));
vi.mock("@agent-management-platform/level-resource-listing", () => ({
  AgentList: () => <div>agent list</div>,
}));
vi.mock("@agent-management-platform/shared-component", () => ({
  OverviewPanels: ({ props }: { props: unknown }) => {
    hoisted.panelProps.push(props);
    return <div>module panels</div>;
  },
}));
vi.mock("./subComponents/AgentTypeSummary", () => ({
  AgentTypeSummary: () => <div>agent type summary</div>,
}));

import { ProjectOverview } from "./ProjectOverview";

const Destination = () => <div>{`at ${useLocation().pathname}`}</div>;

const renderPage = () =>
  render(
    <MemoryRouter
      initialEntries={["/org/acme/project/shop"]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <Routes>
        <Route path="/org/:orgId/project/:projectId" element={<ProjectOverview />} />
        <Route path="*" element={<Destination />} />
      </Routes>
    </MemoryRouter>,
  );

const someAgents = { data: { agents: [{ name: "bot" }] }, isSuccess: true, isFetching: false };
const noAgents = { data: { agents: [] }, isSuccess: true, isFetching: false };

beforeEach(() => {
  hoisted.canWriteProject = true;
  hoisted.canCreateAgent = true;
  hoisted.project = { data: { metadata: { name: "shop" }, spec: { displayName: "Shop", description: "Storefront agents" } }, isLoading: false };
  hoisted.agents = someAgents;
  hoisted.panelProps = [];
});

describe("ProjectOverview", () => {
  it("shows the project's name and description, its agents and the module panels", () => {
    renderPage();

    expect(screen.getByText("Shop")).toBeInTheDocument();
    expect(screen.getByText("Storefront agents")).toBeInTheDocument();
    expect(screen.getByText("agent list")).toBeInTheDocument();
    expect(screen.getByText("agent type summary")).toBeInTheDocument();
    expect(screen.getByText("module panels")).toBeInTheDocument();
    expect(hoisted.panelProps.at(-1)).toEqual({ orgId: "acme", projectId: "shop" });
  });

  it("opens the edit page from Edit project", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Edit project" }));

    expect(screen.getByText("at /org/acme/project/shop/edit")).toBeInTheDocument();
  });

  it("offers no Edit project without write access to projects", () => {
    hoisted.canWriteProject = false;
    renderPage();

    expect(screen.queryByRole("button", { name: "Edit project" })).not.toBeInTheDocument();
  });

  it("sends a project with no agents to the Add a New Agent page", () => {
    hoisted.agents = noAgents;
    renderPage();

    expect(screen.getByText("at /org/acme/project/shop/newAgent")).toBeInTheDocument();
  });

  it("keeps a project with no agents when the user cannot create agents", () => {
    hoisted.agents = noAgents;
    hoisted.canCreateAgent = false;
    renderPage();

    expect(screen.getByText("Shop")).toBeInTheDocument();
  });

  it("stays when the agent list failed to load", () => {
    hoisted.agents = { data: undefined, isSuccess: false, isFetching: false };
    renderPage();

    expect(screen.getByText("Shop")).toBeInTheDocument();
  });

  it("waits for a refetch to finish before redirecting", () => {
    hoisted.agents = { ...noAgents, isFetching: true };
    renderPage();

    expect(screen.getByText("Shop")).toBeInTheDocument();
  });
});

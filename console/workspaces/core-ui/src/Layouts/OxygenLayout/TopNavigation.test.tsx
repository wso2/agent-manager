/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { Header, OxygenUIThemeProvider } from "@wso2/oxygen-ui";
import { TopNavigation } from "./TopNavigation";


const mocks = vi.hoisted(() => ({
  orgs: undefined as unknown,
  projects: undefined as unknown,
  agents: undefined as unknown,
  userInfo: {} as Record<string, unknown>,
}));

vi.mock("@agent-management-platform/auth", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useAuthHooks: () => ({ userInfo: mocks.userInfo }),
}));

vi.mock("@agent-management-platform/api-client", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useListOrganizations: () => ({ data: mocks.orgs }),
  useListProjects: () => ({ data: mocks.projects }),
  useListAgents: () => ({ data: mocks.agents }),
}));

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

function renderAt(path: string) {
  const element = (
    <>
      <Header>
        <TopNavigation />
      </Header>
      <Where />
    </>
  );
  return render(
    <OxygenUIThemeProvider>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/org/:orgId/*" element={element} />
          <Route path="/org/:orgId/project/:projectId/*" element={element} />
          <Route path="/org/:orgId/project/:projectId/agents/:agentId/*" element={element} />
        </Routes>
      </MemoryRouter>
    </OxygenUIThemeProvider>,
  );
}

const twoOrgs = {
  organizations: [
    { name: "ns-acme", handle: "acme", displayName: "Acme Corp" },
    { name: "ns-beta", handle: "beta", displayName: "" },
  ],
  total: 2,
};

beforeEach(() => {
  mocks.orgs = twoOrgs;
  mocks.projects = undefined;
  mocks.agents = undefined;
  mocks.userInfo = {};
});

describe("TopNavigation", () => {
  it("links the selected org and keeps the current org page when switching", () => {
    renderAt("/org/acme/gateways");
    expect(screen.getByText("Organizations")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to Acme Corp", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/gateways",
    );
    fireEvent.click(screen.getByRole("button", { name: "Switch organization", hidden: true }));
    const menu = screen.getByRole("menu", { hidden: true });
    // An org without a display name is listed by its namespace.
    expect(within(menu).getByRole("menuitem", { name: "ns-beta", hidden: true })).toHaveAttribute(
      "href",
      "/org/beta/gateways",
    );
    fireEvent.click(within(menu).getByRole("menuitem", { name: "ns-beta", hidden: true }));
    expect(screen.getByTestId("where")).toHaveTextContent("/org/beta/gateways");
  });

  it("drops project-scoped pages from the org link", () => {
    renderAt("/org/acme/project/p1");
    expect(screen.getByRole("link", { name: "Go to Acme Corp", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme",
    );
  });

  it("names a single org from the user's claims when it has no display name", () => {
    mocks.orgs = { organizations: [{ name: "ns-solo", displayName: "" }], total: 1 };
    mocks.userInfo = { ouName: "Solo Org" };
    renderAt("/org/ns-solo");
    expect(screen.getByText("Organization")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Go to Solo Org", hidden: true })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Switch organization", hidden: true })).not.toBeInTheDocument();
  });

  it("hides the org card for an org not in the list", () => {
    renderAt("/org/unknown");
    expect(screen.queryByText("Organizations")).not.toBeInTheDocument();
  });

  it("offers to select or create a project when none is selected", () => {
    mocks.projects = { list: [{ metadata: { name: "p1" }, spec: { displayName: "Project One" } }] };
    renderAt("/org/acme/projects");
    fireEvent.click(screen.getByRole("button", { name: "Select or create a project", hidden: true }));
    const menu = screen.getByRole("menu", { hidden: true });
    expect(within(menu).getByRole("menuitem", { name: /Create a Project/, hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/newProject",
    );
    expect(within(menu).getByRole("menuitem", { name: "Project One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1",
    );
  });

  it("shows the selected project and closes it back to the listing", () => {
    mocks.projects = {
      list: [
        { metadata: { name: "p1" }, spec: { displayName: "Project One" } },
        { metadata: { name: "p2" }, spec: { displayName: "Project Two" } },
      ],
    };
    renderAt("/org/acme/project/p1/settings");
    expect(screen.getByRole("link", { name: "Go to Project One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/settings",
    );
    fireEvent.click(screen.getByRole("button", { name: "Switch project", hidden: true }));
    expect(screen.getByRole("menuitem", { name: "Project Two", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p2/settings",
    );
    fireEvent.keyDown(screen.getByRole("menu", { hidden: true }), { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Close project", hidden: true }));
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/projects");
  });

  it("keeps the agent listing when switching projects from it", () => {
    mocks.projects = {
      list: [
        { metadata: { name: "p1" }, spec: { displayName: "Project One" } },
        { metadata: { name: "p2" }, spec: { displayName: "Project Two" } },
      ],
    };
    mocks.agents = { agents: [] };
    renderAt("/org/acme/project/p1/agents");
    expect(screen.getByRole("link", { name: "Go to Project One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents",
    );
  });

  it("shows the selected agent, switches agents on the same page, and closes it", () => {
    mocks.projects = { list: [{ metadata: { name: "p1" }, spec: { displayName: "Project One" } }] };
    mocks.agents = {
      agents: [
        { name: "a1", displayName: "Agent One" },
        { name: "a2", displayName: "Agent Two" },
      ],
    };
    renderAt("/org/acme/project/p1/agents/a1/builds");
    expect(screen.getByRole("link", { name: "Go to Agent One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents/a1/builds",
    );
    // Agent-detail pages are not carried over to the project link.
    expect(screen.getByRole("link", { name: "Go to Project One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1",
    );
    fireEvent.click(screen.getByRole("button", { name: "Switch agent", hidden: true }));
    const menu = screen.getByRole("menu", { hidden: true });
    expect(within(menu).getByRole("menuitem", { name: /Create an Agent/, hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/newAgent",
    );
    expect(within(menu).getByRole("menuitem", { name: "Agent Two", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents/a2/builds",
    );
    fireEvent.keyDown(menu, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Close agent", hidden: true }));
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/project/p1");
  });

  it("does not carry environment pages over when switching agents", () => {
    mocks.projects = { list: [{ metadata: { name: "p1" }, spec: { displayName: "Project One" } }] };
    mocks.agents = { agents: [{ name: "a1", displayName: "Agent One" }] };
    renderAt("/org/acme/project/p1/agents/a1/environment/dev");
    expect(screen.getByRole("link", { name: "Go to Agent One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents/a1",
    );
  });

  it("offers to select or create an agent when none is selected", () => {
    mocks.projects = { list: [{ metadata: { name: "p1" }, spec: { displayName: "Project One" } }] };
    mocks.agents = { agents: [{ name: "a1", displayName: "Agent One" }] };
    renderAt("/org/acme/project/p1");
    expect(screen.getByRole("button", { name: "Select or create an agent", hidden: true })).toBeInTheDocument();
  });
});

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
  const location = useLocation();
  return (
    <div data-testid="where" data-from={(location.state as { from?: string } | null)?.from ?? ""}>
      {location.pathname}
    </div>
  );
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
    expect(screen.getAllByText("Organization").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Acme Corp", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/gateways",
    );
    fireEvent.click(screen.getByRole("button", { name: "Switch organization", hidden: true }));
    const menu = screen.getAllByRole("menu", { hidden: true })[0];
    // An org without a display name is listed by its namespace.
    expect(within(menu).getByRole("menuitemradio", { name: "ns-beta", hidden: true })).toHaveAttribute(
      "href",
      "/org/beta/gateways",
    );
    fireEvent.click(within(menu).getByRole("menuitemradio", { name: "ns-beta", hidden: true }));
    expect(screen.getByTestId("where")).toHaveTextContent("/org/beta/gateways");
  });

  it("drops project-scoped pages from the org link", () => {
    renderAt("/org/acme/project/p1");
    expect(screen.getByRole("button", { name: "Acme Corp", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme",
    );
  });

  it("names a single org from the user's claims when it has no display name", () => {
    mocks.orgs = { organizations: [{ name: "ns-solo", displayName: "" }], total: 1 };
    mocks.userInfo = { ouName: "Solo Org" };
    renderAt("/org/ns-solo");
    expect(screen.getAllByText("Organization").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Solo Org", hidden: true })).toBeInTheDocument();
  });

  it("hides the org card for an org not in the list", () => {
    renderAt("/org/unknown");
    expect(screen.queryAllByText("Organization")).toHaveLength(0);//.toBeInTheDocument();
  });

  it("offers to select or create a project when none is selected", () => {
    mocks.projects = { projects: [{ name: "p1", displayName: "Project One" }] };
    renderAt("/org/acme/projects");
    fireEvent.click(screen.getByRole("button", { name: "Select or create a project", hidden: true }));
    const menu = screen.getAllByRole("menu", { hidden: true })[0];
    expect(screen.getByRole("menuitem", { name: /Create a Project/, hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/newProject",
    );
    expect(within(menu).getByRole("menuitemradio", { name: "Project One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1",
    );
    // The create page is told where it was opened from, so Cancel/Save return here.
    fireEvent.click(screen.getByRole("menuitem", { name: /Create a Project/, hidden: true }));
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/newProject");
    expect(screen.getByTestId("where")).toHaveAttribute("data-from", "/org/acme/projects");
  });

  it("shows the selected project and switches projects on the same page", () => {
    mocks.projects = {
      projects: [
        { name: "p1", displayName: "Project One" },
        { name: "p2", displayName: "Project Two" },
      ],
    };
    renderAt("/org/acme/project/p1/settings");
    expect(screen.getByRole("button", { name: "Project One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/settings",
    );
    fireEvent.click(screen.getByRole("button", { name: "Switch project", hidden: true }));
    expect(screen.getByRole("menuitemradio", { name: "Project Two", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p2/settings",
    );
  });

  it("keeps the agent listing when switching projects from it", () => {
    mocks.projects = {
      projects: [
        { name: "p1", displayName: "Project One" },
        { name: "p2", displayName: "Project Two" },
      ],
    };
    mocks.agents = { agents: [] };
    renderAt("/org/acme/project/p1/agents");
    expect(screen.getByRole("button", { name: "Project One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents",
    );
  });

  it("shows the selected agent and switches agents on the same page", () => {
    mocks.projects = { projects: [{ name: "p1", displayName: "Project One" }] };
    mocks.agents = {
      agents: [
        { name: "a1", displayName: "Agent One" },
        { name: "a2", displayName: "Agent Two" },
      ],
    };
    renderAt("/org/acme/project/p1/agents/a1/builds");
    expect(screen.getByRole("button", { name: "Agent One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents/a1/builds",
    );
    // Agent-detail pages are not carried over to the project link.
    expect(screen.getByRole("button", { name: "Project One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1",
    );
    fireEvent.click(screen.getByRole("button", { name: "Switch agent", hidden: true }));
    const menu = screen.getAllByRole("menu", { hidden: true })[0];
    expect(screen.getByRole("menuitem", { name: /Create an Agent/, hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/newAgent",
    );
    expect(within(menu).getByRole("menuitemradio", { name: "Agent Two", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents/a2/builds",
    );
  });

  it("does not carry environment pages over when switching agents", () => {
    mocks.projects = { projects: [{ name: "p1", displayName: "Project One" }] };
    mocks.agents = { agents: [{ name: "a1", displayName: "Agent One" }] };
    renderAt("/org/acme/project/p1/agents/a1/environment/dev");
    expect(screen.getByRole("button", { name: "Agent One", hidden: true })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents/a1",
    );
  });

  it("offers to select or create an agent when none is selected", () => {
    mocks.projects = { projects: [{ name: "p1", displayName: "Project One" }] };
    mocks.agents = { agents: [{ name: "a1", displayName: "Agent One" }] };
    renderAt("/org/acme/project/p1");
    expect(screen.getByRole("button", { name: "Select or create an agent", hidden: true })).toBeInTheDocument();
  });
});

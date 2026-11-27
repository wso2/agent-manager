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

import { render, screen, waitFor } from "@testing-library/react";
import { Outlet } from "react-router-dom";
import { OxygenUIThemeProvider } from "@wso2/oxygen-ui";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import {
  PLUGIN_API_VERSION,
  type PluginActivationState,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import type { ExternalModule } from "@agent-management-platform/views";
import { PluginProvider } from "../plugins";
import { MountPoints } from "../types";
import { RootRouter } from "./Route";


type Query = { data?: unknown; isLoading: boolean; isError?: boolean; error?: unknown };

const mocks = vi.hoisted(() => ({
  auth: {} as Record<string, unknown>,
  orgs: {} as Query,
  project: {} as Query,
  agent: {} as Query,
  projects: {} as Query,
  pageViews: [] as string[],
  activation: undefined as PluginActivationState | "unset",
}));

vi.mock("@agent-management-platform/auth", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useAuthHooks: () => mocks.auth,
  isFileAuthMode: false,
}));

// Stable like a real query result: a new object per render would loop the
// manifest effect in PluginActivators.
const uiManifest = vi.hoisted(() => ({
  data: { pages: [] as string[] },
  isError: false,
  isPending: false,
}));

vi.mock("@agent-management-platform/api-client", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useListOrganizations: () => mocks.orgs,
  useListProjects: () => mocks.projects,
  useGetProject: () => mocks.project,
  useGetAgent: () => mocks.agent,
  usePageViewTracking: (pathname: string) => {
    mocks.pageViews.push(pathname);
  },
  useSessionAnalytics: () => {},
  useUIManifest: () => uiManifest,
}));

// The real layout has its own tests; here it only has to render the page.
vi.mock("../Layouts", () => ({
  OxygenLayout: () => (
    <div data-testid="layout">
      <Outlet />
    </div>
  ),
}));

const page = (text: string) => () => Promise.resolve({ default: () => <div>{text}</div> });

const testModule: UIModuleManifest = {
  id: "test",
  name: "Test",
  description: "",
  apiVersion: PLUGIN_API_VERSION,
  core: true,
  plugins: [
    {
      id: "pages",
      name: "Pages",
      apiVersion: PLUGIN_API_VERSION,
      contributions: [
        { type: "route", id: "org-home", level: "org", index: true, load: page("org home") },
        { type: "route", id: "proj-home", level: "project", index: true, load: page("project home") },
        { type: "route", id: "agent-home", level: "agent", index: true, load: page("agent home") },
      ],
    },
    {
      id: "gated",
      name: "Gated",
      apiVersion: PLUGIN_API_VERSION,
      useActivation: () => (mocks.activation === "unset" ? undefined : mocks.activation),
      contributions: [
        { type: "route", id: "gated", level: "org", path: "gated", load: page("gated page") },
      ],
    },
  ],
};

const externalModules: ExternalModule[] = [
  {
    kind: "page",
    mountPoint: MountPoints.OrgLevelPage,
    moduleName: "ext-org",
    path: "ext-org",
    pageComponent: () => <div>external org page</div>,
  },
  {
    kind: "page",
    mountPoint: MountPoints.ProjectLevelPage,
    moduleName: "ext-project",
    path: "ext-project",
    pageComponent: () => <div>external project page</div>,
  },
  {
    kind: "page",
    mountPoint: MountPoints.ComponentLevelPage,
    moduleName: "ext-agent",
    path: "ext-agent",
    pageComponent: () => <div>external agent page</div>,
  },
];

const runtimeConfig = (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> })
  .__RUNTIME_CONFIG__;

function renderAt(path: string) {
  window.history.pushState({}, "", path);
  return render(
    <OxygenUIThemeProvider>
      <ConfirmationDialogProvider>
        <PluginProvider modules={[testModule]} externalModules={externalModules}>
          <RootRouter />
        </PluginProvider>
      </ConfirmationDialogProvider>
    </OxygenUIThemeProvider>,
  );
}

const login = vi.fn();

beforeEach(() => {
  login.mockReset();
  mocks.pageViews.length = 0;
  mocks.activation = true;
  mocks.auth = {
    isAuthenticated: true,
    isLoadingIsAuthenticated: false,
    isLoadingUserInfo: false,
    userInfo: { sub: "u1" },
    login,
    logout: vi.fn(),
    getToken: vi.fn(),
  };
  mocks.orgs = {
    data: { organizations: [{ name: "ns-acme", handle: "acme", displayName: "Acme" }] },
    isLoading: false,
    error: null,
  };
  mocks.projects = { data: { list: [{ metadata: { name: "default" } }] }, isLoading: false };
  mocks.project = { data: {}, isLoading: false, isError: false };
  mocks.agent = { data: {}, isLoading: false, isError: false };
  delete runtimeConfig.uiModules;
});

describe("RootRouter", () => {
  it("renders the login page at /login", async () => {
    mocks.auth = { ...mocks.auth, isAuthenticated: false, userInfo: undefined };
    renderAt("/login");
    await waitFor(() => expect(login).toHaveBeenCalled());
    expect(screen.queryByTestId("layout")).not.toBeInTheDocument();
  });

  it("sends signed-out users from a console page to the login page", async () => {
    mocks.auth = { ...mocks.auth, isAuthenticated: false, userInfo: undefined };
    renderAt("/org/acme");
    await waitFor(() => expect(window.location.pathname).toBe("/login"));
    expect(screen.queryByText("org home")).not.toBeInTheDocument();
  });

  it("renders the org's plugin page when the org exists and reports the page view", async () => {
    renderAt("/org/acme");
    expect(await screen.findByText("org home")).toBeInTheDocument();
    expect(mocks.pageViews).toContain("/org/acme");
  });

  it("shows a not-found page for an org the user cannot see", async () => {
    renderAt("/org/other");
    expect(await screen.findByText("Organization Not Found")).toBeInTheDocument();
    expect(
      screen.getByText(`The organization "other" doesn't exist or you don't have access to it.`),
    ).toBeInTheDocument();
  });

  it("shows a load failure, not a not-found page, when the org list fails", async () => {
    mocks.orgs = { data: { organizations: [{ name: "ns-acme", handle: "acme" }] }, isLoading: false, error: new Error("x") };
    renderAt("/org/acme");
    // Protected surfaces the error first.
    expect(await screen.findByText("Failed to Load Organizations")).toBeInTheDocument();
  });

  it("moves links that name the org by namespace onto its handle", async () => {
    renderAt("/org/ns-acme/project/default?tab=1#top");
    await waitFor(() => expect(window.location.pathname).toBe("/org/acme/project/default"));
    expect(window.location.search).toBe("?tab=1");
    expect(window.location.hash).toBe("#top");
    expect(await screen.findByText("project home")).toBeInTheDocument();
  });

  it.each([
    ["/org/acme/identities", "/org/acme/settings/identities"],
    ["/org/acme/identities/users/u1", "/org/acme/settings/identities/users/u1"],
  ])("redirects the old identities link %s", async (from, to) => {
    renderAt(from);
    await waitFor(() => expect(window.location.pathname).toBe(to));
  });

  it("shows a loader while the project loads, then the project page", async () => {
    mocks.project = { isLoading: true, isError: false };
    const view = renderAt("/org/acme/project/p1");
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
    mocks.project = { data: {}, isLoading: false, isError: false };
    view.rerender(
      <OxygenUIThemeProvider>
        <ConfirmationDialogProvider>
          <PluginProvider modules={[testModule]} externalModules={externalModules}>
            <RootRouter />
          </PluginProvider>
        </ConfirmationDialogProvider>
      </OxygenUIThemeProvider>,
    );
    expect(await screen.findByText("project home")).toBeInTheDocument();
  });

  it("shows a not-found page for a missing project", async () => {
    mocks.project = { isLoading: false, isError: true };
    renderAt("/org/acme/project/p1");
    expect(await screen.findByText("Project Not Found")).toBeInTheDocument();
    expect(screen.queryByText("project home")).not.toBeInTheDocument();
  });

  it("renders the agent page when the agent exists", async () => {
    renderAt("/org/acme/project/p1/agents/a1");
    expect(await screen.findByText("agent home")).toBeInTheDocument();
  });

  it("shows a not-found page for a missing agent", async () => {
    mocks.agent = { isLoading: false, isError: true };
    renderAt("/org/acme/project/p1/agents/a1");
    expect(await screen.findByText("Agent Not Found")).toBeInTheDocument();
    expect(
      screen.getByText(`The agent "a1" doesn't exist or you don't have access to it.`),
    ).toBeInTheDocument();
  });

  it.each([
    "/org/acme/nope",
    "/org/acme/project/p1/nope",
    "/org/acme/project/p1/agents/a1/nope",
  ])("shows the 404 page for unknown path %s", async (path) => {
    renderAt(path);
    expect(await screen.findByText("Page Not Found")).toBeInTheDocument();
  });

  it.each([
    ["/org/acme/ext-org/deep", "external org page"],
    ["/org/acme/project/p1/ext-project", "external project page"],
    ["/org/acme/project/p1/agents/a1/ext-agent", "external agent page"],
  ])("mounts legacy external page modules at %s", async (path, text) => {
    renderAt(path);
    expect(await screen.findByText(text)).toBeInTheDocument();
  });

  it("shows a loader for a plugin page whose activation is pending", async () => {
    mocks.activation = "unset";
    renderAt("/org/acme/gated");
    expect(screen.getByRole("progressbar")).toBeInTheDocument();
    expect(screen.queryByText("Page Not Found")).not.toBeInTheDocument();
  });

  it("has no route for a plugin page whose activation said no", async () => {
    mocks.activation = false;
    renderAt("/org/acme/gated");
    expect(await screen.findByText("Page Not Found")).toBeInTheDocument();
  });

  it("serves the AI module manager only when the runtime config turns it on", async () => {
    renderAt("/org/acme/ui-modules");
    expect(await screen.findByText("Page Not Found")).toBeInTheDocument();

    runtimeConfig.uiModules = { marketplace: true };
    renderAt("/org/acme/ui-modules");
    expect(await screen.findByText("AI module manager")).toBeInTheDocument();
  });
});

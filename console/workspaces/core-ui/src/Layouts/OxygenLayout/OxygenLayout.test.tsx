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

import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { OxygenUIThemeProvider } from "@wso2/oxygen-ui";
import {
  PLUGIN_API_VERSION,
  type PluginNavItemContribution,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import type { ExternalModule } from "@agent-management-platform/views";
import { PluginActivators, PluginProvider } from "../../plugins";
import { MountPoints } from "../../types";
import { OxygenLayout } from "./OxygenLayout";


const mocks = vi.hoisted(() => ({
  auth: {} as Record<string, unknown>,
  flags: {} as Record<string, boolean>,
  agent: {} as { data?: unknown; isLoading: boolean },
  envs: {} as { environments: { name: string }[]; isLoading: boolean },
  profile: undefined as unknown,
  profileOptions: [] as unknown[],
  canManageSecrets: true,
}));

vi.mock("@agent-management-platform/auth", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useAuthHooks: () => mocks.auth,
}));

vi.mock("@agent-management-platform/feature-management", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useFeatureFlag: (key: string) => !!mocks.flags[key],
}));

vi.mock("@agent-management-platform/shared-component", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useOrgEnvironmentsState: () => ({ ...mocks.envs, isError: false }),
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
  useUIManifest: () => uiManifest,
  useRouteAgent: () => mocks.agent,
  useGetUserProfile: (_params: unknown, options?: unknown) => {
    mocks.profileOptions.push(options);
    return { data: mocks.profile };
  },
  useUpdateUserProfile: () => ({ mutateAsync: vi.fn(), isPending: false, error: null }),
  useListOrganizations: () => ({
    data: { organizations: [{ name: "ns-acme", handle: "acme", displayName: "Acme" }], total: 1 },
  }),
  useListProjects: () => ({ data: undefined }),
  useListAgents: () => ({ data: undefined }),
}));

const Icon = () => <svg />;

function nav(
  id: string,
  level: PluginNavItemContribution["level"],
  path: string,
  extra: Partial<PluginNavItemContribution> = {},
): PluginNavItemContribution {
  return {
    type: "nav-item",
    id,
    level,
    label: extra.label ?? id,
    icon: Icon,
    route: { path, wildPath: `${path}/*` },
    ...extra,
  };
}

const coreModule: UIModuleManifest = {
  id: "core",
  name: "Core",
  description: "",
  apiVersion: PLUGIN_API_VERSION,
  core: true,
  plugins: [
    {
      id: "core-nav",
      name: "Core nav",
      apiVersion: PLUGIN_API_VERSION,
      contributions: [
        nav("Overview", "org", "/org/:orgId/overview", { section: "main", order: 1 }),
        nav("Projects", "org", "/org/:orgId/projects", { section: "main", order: 2 }),
        nav("Settings", "org", "/org/:orgId/settings", { section: "footer", pinBottom: true }),
        nav("Help", "org", "/org/:orgId/help", { section: "footer", dividerBefore: true }),
        nav("Gateways", "org", "/org/:orgId/gateways", { section: "resources" }),
        nav("Orphan", "org", "/org/:orgId/orphan", { section: "no-such-slot" }),
        nav("Secrets", "org", "/org/:orgId/secrets", {
          section: "resources",
          // Stands in for a permission check: hidden when the user lacks it.
          useActivation: () => mocks.canManageSecrets,
        }),
        nav("Agents", "project", "/org/:orgId/project/:projectId/agents", { section: "main" }),
        nav("Deploy", "agent", "/org/:orgId/project/:projectId/agents/:agentId/env/:envId", {
          section: "lifecycle",
        }),
        nav("Agent overview", "agent", "/org/:orgId/project/:projectId/agents/:agentId", {
          section: "main",
          activePaths: ["/org/:orgId/project/:projectId/agents/:agentId"],
        }),
      ],
    },
  ],
};

// A non-core module the server manifest leaves out contributes nothing.
const offModule: UIModuleManifest = {
  id: "observability",
  name: "Observability",
  description: "",
  apiVersion: PLUGIN_API_VERSION,
  plugins: [
    {
      id: "obs",
      name: "Obs",
      apiVersion: PLUGIN_API_VERSION,
      contributions: [nav("Traces", "org", "/org/:orgId/traces", { section: "main" })],
    },
  ],
};

const panel = (mountPoint: string, text: string): ExternalModule => ({
  kind: "component",
  mountPoint,
  moduleName: text,
  component: () => <span>{text}</span>,
});

const runtimeConfig = (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> })
  .__RUNTIME_CONFIG__;

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

function renderAt(path: string, externalModules: ExternalModule[] = []) {
  return render(
    <OxygenUIThemeProvider>
      <MemoryRouter initialEntries={[path]}>
        <PluginProvider modules={[coreModule, offModule]} externalModules={externalModules}>
          <PluginActivators />
          <Routes>
            {[
              "/org/:orgId/*",
              "/org/:orgId/project/:projectId/*",
              "/org/:orgId/project/:projectId/agents/:agentId/*",
              "/elsewhere",
            ].map((p) => (
              <Route key={p} path={p} element={<OxygenLayout />}>
                <Route path="*" element={<Where />} />
              </Route>
            ))}
          </Routes>
        </PluginProvider>
      </MemoryRouter>
    </OxygenUIThemeProvider>,
  );
}

beforeEach(() => {
  mocks.auth = {
    userInfo: { sub: "u1", givenName: "Jane", familyName: "Doe", email: "jane@example.com" },
    logout: vi.fn().mockResolvedValue(undefined),
  };
  mocks.flags = {};
  mocks.agent = { data: undefined, isLoading: false };
  mocks.envs = { environments: [{ name: "dev" }], isLoading: false };
  mocks.profile = undefined;
  mocks.canManageSecrets = true;
  delete runtimeConfig.uiModules;
  delete runtimeConfig.ampVersion;
  delete runtimeConfig.docsUrl;
  delete runtimeConfig.footerLinks;
});

describe("OxygenLayout profile", () => {
  // The layout only shows the profile's picture, so a missing profile must not
  // pop an error over a page that loaded fine.
  it("reads the profile silently", () => {
    mocks.profileOptions.length = 0;
    renderAt("/org/acme/overview", []);
    expect(mocks.profileOptions.length).toBeGreaterThan(0);
    const silent = mocks.profileOptions.map((o) => (o as { silent?: boolean })?.silent);
    expect(silent.every((s) => s === true)).toBe(true);
  });
});

describe("OxygenLayout navigation", () => {
  it("lays out the org menu by slot and renders the page", () => {
    renderAt("/org/acme/overview", [
      {
        kind: "nav-item",
        mountPoint: "nav",
        moduleName: "legacy",
        level: "org",
        title: "Legacy page",
        route: "/org/:orgId/legacy",
        icon: <Icon />,
      },
      {
        kind: "nav-item",
        mountPoint: "nav",
        moduleName: "legacy-project",
        level: "project",
        title: "Legacy project page",
        route: "/org/:orgId/project/:projectId/legacy",
        icon: <Icon />,
      },
    ]);
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/overview");
    expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute(
      "href",
      "/org/acme/overview",
    );
    expect(screen.getByText("Resources")).toBeInTheDocument();
    for (const label of ["Projects", "Settings", "Help", "Gateways", "Orphan", "Secrets"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
    expect(screen.getByRole("link", { name: "Legacy page" })).toHaveAttribute(
      "href",
      "/org/acme/legacy",
    );
    expect(screen.queryByText("Legacy project page")).not.toBeInTheDocument();
    // Other levels' items stay out of the org menu.
    expect(screen.queryByText("Agents")).not.toBeInTheDocument();
  });

  it("hides nav items the user may not see and modules that are off", () => {
    mocks.canManageSecrets = false;
    renderAt("/org/acme/overview");
    expect(screen.getByText("Gateways")).toBeInTheDocument();
    expect(screen.queryByText("Secrets")).not.toBeInTheDocument();
    expect(screen.queryByText("Traces")).not.toBeInTheDocument();
  });

  it("shows the project menu inside a project", () => {
    renderAt("/org/acme/project/p1/agents");
    expect(screen.getByRole("link", { name: "Agents" })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents",
    );
    expect(screen.queryByText("Overview")).not.toBeInTheDocument();
  });

  it("shows the agent menu with the first environment in its links", () => {
    mocks.agent = { data: { provisioning: { type: "internal" } }, isLoading: false };
    renderAt("/org/acme/project/p1/agents/a1");
    expect(screen.getByText("Agent lifecycle")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Deploy" })).toHaveAttribute(
      "href",
      "/org/acme/project/p1/agents/a1/env/dev",
    );
  });

  it("falls back to the project menu for an internal agent with no environment", () => {
    mocks.agent = { data: { provisioning: { type: "internal" } }, isLoading: false };
    mocks.envs = { environments: [], isLoading: false };
    renderAt("/org/acme/project/p1/agents/a1");
    expect(screen.getByText("Agents")).toBeInTheDocument();
    expect(screen.queryByText("Deploy")).not.toBeInTheDocument();
  });

  it("gives external agents the agent menu without an environment", () => {
    mocks.agent = { data: { provisioning: { type: "external" } }, isLoading: false };
    mocks.envs = { environments: [], isLoading: false };
    renderAt("/org/acme/project/p1/agents/a1");
    expect(screen.getByText("Agent overview")).toBeInTheDocument();
  });

  it("shows no menu while the agent loads", () => {
    mocks.agent = { isLoading: true };
    renderAt("/org/acme/project/p1/agents/a1");
    expect(screen.queryByText("Agent overview")).not.toBeInTheDocument();
    expect(screen.queryByText("Agents")).not.toBeInTheDocument();
  });

  it("shows no menu outside an org", () => {
    renderAt("/elsewhere");
    expect(screen.queryByText("Overview")).not.toBeInTheDocument();
  });

  it("collapses and expands the sidebar", () => {
    renderAt("/org/acme/overview");
    fireEvent.click(screen.getByText("Collapse"));
    expect(screen.getByText("Expand")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Expand"));
    expect(screen.getByText("Collapse")).toBeInTheDocument();
  });

  it("toggles a collapsible section", () => {
    renderAt("/org/acme/overview");
    const section = screen.getByText("Resources").closest("[aria-expanded]");
    expect(section).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(screen.getByText("Resources"));
    expect(screen.getByText("Resources").closest("[aria-expanded]")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
  });
});

describe("OxygenLayout header and footer", () => {
  it("shows the user's name and initials, and signs out", async () => {
    renderAt("/org/acme/overview");
    expect(screen.getAllByText("JD").length).toBeGreaterThan(0);
    fireEvent.click(screen.getAllByText("JD")[0]);
    expect(await screen.findByText("jane@example.com")).toBeInTheDocument();
    expect(screen.queryByText("Profile settings")).not.toBeInTheDocument();
    expect(screen.queryByText("AI module manager")).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Sign out"));
    expect(mocks.auth.logout).toHaveBeenCalled();
  });

  it("uses the profile picture when there is one", () => {
    mocks.profile = { attributes: { picture: "https://example.com/me.png" } };
    renderAt("/org/acme/overview");
    expect(screen.queryByText("JD")).not.toBeInTheDocument();
    expect(document.querySelector('img[src="https://example.com/me.png"]')).not.toBeNull();
  });

  it("falls back to the username, then a generic user", () => {
    mocks.auth = { ...mocks.auth, userInfo: { username: "jdoe" } };
    const view = renderAt("/org/acme/overview");
    expect(screen.getAllByText("J").length).toBeGreaterThan(0);
    view.unmount();
    mocks.auth = { ...mocks.auth, userInfo: undefined };
    renderAt("/org/acme/overview");
    expect(screen.getAllByText("U").length).toBeGreaterThan(0);
  });

  it("offers profile settings and the module manager when turned on", async () => {
    mocks.flags = { enableProfileManagement: true };
    runtimeConfig.uiModules = { marketplace: true };
    renderAt("/org/acme/overview");
    fireEvent.click(screen.getAllByText("JD")[0]);
    fireEvent.click(await screen.findByText("Profile settings"));
    expect(await screen.findByText("Profile Settings")).toBeInTheDocument();

    fireEvent.click(screen.getAllByText("JD")[0]);
    fireEvent.click(await screen.findByText("AI module manager"));
    expect(screen.getByTestId("where")).toHaveTextContent("/org/acme/ui-modules");
  });

  it("goes home to the first org from the brand", () => {
    renderAt("/org/acme/project/p1/agents", [panel(MountPoints.TopLogo, "custom logo")]);
    fireEvent.click(screen.getByText("custom logo"));
    expect(screen.getByTestId("where")).toHaveTextContent(/^\/org\/acme$/);
  });

  it("renders header mount-point panels and replaces the logo", () => {
    renderAt("/org/acme/overview", [
      panel(MountPoints.TopStartPanel, "top start"),
      panel(MountPoints.TopLogo, "custom logo"),
      panel(MountPoints.TopLeftPanel, "top left"),
      panel(MountPoints.TopRightPanel, "top right"),
    ]);
    for (const text of ["top start", "custom logo", "top left", "top right"]) {
      expect(screen.getByText(text)).toBeInTheDocument();
    }
  });

  it("shows the version and configured footer links", () => {
    runtimeConfig.ampVersion = "v9.9.9";
    runtimeConfig.docsUrl = "https://docs.example.com";
    runtimeConfig.footerLinks = {
      supportUrl: "https://support.example.com",
      termsOfUseUrl: "https://terms.example.com",
      privacyPolicyUrl: "https://privacy.example.com",
    };
    renderAt("/org/acme/overview");
    expect(screen.getByText("v9.9.9")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Documentation" })).toHaveAttribute(
      "href",
      "https://docs.example.com/get-started/what-is-amp/",
    );
    expect(screen.getByRole("link", { name: "Support" })).toHaveAttribute(
      "href",
      "https://support.example.com",
    );
    expect(screen.getByRole("link", { name: "Terms of Use" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Privacy Policy" })).toBeInTheDocument();
  });

  it("renders footer mount-point panels", () => {
    runtimeConfig.docsUrl = "https://docs.example.com";
    renderAt("/org/acme/overview", [
      panel(MountPoints.BottomLeftPanel, "bottom left"),
      panel(MountPoints.BottomRightPanel, "bottom right"),
    ]);
    expect(screen.getByText("bottom left")).toBeInTheDocument();
    expect(screen.getByText("bottom right")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Documentation" })).toBeInTheDocument();
  });

  it("leaves footer links out when they are not configured", () => {
    renderAt("/org/acme/overview");
    expect(screen.queryByRole("link", { name: "Documentation" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Support" })).not.toBeInTheDocument();
  });
});


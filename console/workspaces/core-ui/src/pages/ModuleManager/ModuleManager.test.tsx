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

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import { OxygenUIThemeProvider } from "@wso2/oxygen-ui";
import { ConfirmationDialogProvider } from "@agent-management-platform/shared-component";
import { PLUGIN_API_VERSION, type UIModuleManifest } from "@agent-management-platform/types";
import { PluginActivators, PluginProvider, type UIModuleStatus } from "../../plugins";
import { ModuleManager } from "./ModuleManager";
import { ModuleSetup } from "./ModuleSetup";
import { isMarketplaceEnabled, MODULE_MANAGER_ROUTE, MODULE_SETUP_ROUTE } from "./routes";
import { countLabel, listingFor, marketplaceState } from "./moduleUi";


// Stable like a real query result: a new object per render would loop the
// manifest effect in PluginActivators.
const uiManifest = vi.hoisted(() => ({
  data: { pages: ["ai-gateway/gw-pages"] },
  isError: false,
  isPending: false,
}));

vi.mock("@agent-management-platform/api-client", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useUIManifest: () => uiManifest,
}));

const Icon = () => <svg />;
const load = () => Promise.resolve({ default: () => <div>module setup content</div> });

const modules: UIModuleManifest[] = [
  {
    id: "acp-core",
    name: "Agent Platform",
    description: "Projects and agents",
    apiVersion: PLUGIN_API_VERSION,
    core: true,
    plugins: [{ id: "core-pages", name: "Core", apiVersion: PLUGIN_API_VERSION, contributions: [] }],
  },
  {
    id: "ai-gateway",
    name: "AI Gateway",
    description: "Route LLM traffic",
    apiVersion: PLUGIN_API_VERSION,
    installation: {
      chart: "wso2-amp-gateway-extension",
      steps: [
        {
          title: "Install the chart",
          description: "Run this on the cluster.",
          command: "helm install gw oci://{chartRegistry}/gw --version {chartVersion}",
        },
        { title: "Verify" },
      ],
    },
    setup: load,
    plugins: [
      {
        id: "gw-pages",
        name: "Gateway pages",
        apiVersion: PLUGIN_API_VERSION,
        contributions: [
          { type: "route", id: "r", level: "org", path: "gateways", load },
          {
            type: "nav-item",
            id: "n1",
            level: "org",
            label: "Gateways",
            icon: Icon,
            route: { path: "/org/:orgId/gateways", wildPath: "/org/:orgId/gateways/*" },
          },
          {
            type: "nav-item",
            id: "n2",
            level: "org",
            label: "Providers",
            icon: Icon,
            route: { path: "/org/:orgId/providers", wildPath: "/org/:orgId/providers/*" },
          },
          { type: "panel", id: "p", mountPoint: "somewhere", load },
        ],
      },
    ],
  },
  {
    id: "agent-identity",
    name: "Agent Identity",
    description: "Identities for agents",
    apiVersion: PLUGIN_API_VERSION,
    plugins: [{ id: "id-pages", name: "Identity", apiVersion: PLUGIN_API_VERSION, contributions: [] }],
  },
];

const runtimeConfig = (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> })
  .__RUNTIME_CONFIG__;

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

function renderAt(path: string, list = modules) {
  return render(
    <OxygenUIThemeProvider>
      <ConfirmationDialogProvider>
        <MemoryRouter initialEntries={[path]}>
          <PluginProvider modules={list}>
            <PluginActivators />
            <Routes>
              <Route path={MODULE_MANAGER_ROUTE} element={<ModuleManager />} />
              <Route path={MODULE_SETUP_ROUTE} element={<ModuleSetup />} />
              <Route path="*" element={<Where />} />
            </Routes>
          </PluginProvider>
        </MemoryRouter>
      </ConfirmationDialogProvider>
    </OxygenUIThemeProvider>,
  );
}

function card(name: string) {
  return screen.getByRole("heading", { name }).closest(".MuiCard-root") as HTMLElement;
}

beforeEach(() => {
  delete runtimeConfig.uiModules;
  delete runtimeConfig.ampChartRegistry;
  delete runtimeConfig.chartVersion;
});

describe("ModuleManager", () => {
  it("lists every module with its state and action", () => {
    renderAt("/org/acme/ui-modules");
    expect(screen.getByText("AI module manager")).toBeInTheDocument();

    const core = card("Agent Platform");
    expect(within(core).getByText("Built in")).toBeInTheDocument();
    expect(within(core).queryByRole("button", { name: /install/i })).not.toBeInTheDocument();
    expect(within(core).getByText("Platform")).toBeInTheDocument();

    const gateway = card("AI Gateway");
    expect(within(gateway).getByText("Installed")).toBeInTheDocument();
    expect(within(gateway).getByRole("button", { name: "Uninstall" })).toBeInTheDocument();

    // The server manifest lists none of its pages, so the manager cannot turn it on.
    const identity = card("Agent Identity");
    expect(within(identity).getByText("Turned off by admin")).toBeInTheDocument();
    expect(within(identity).queryByRole("button", { name: /install/i })).not.toBeInTheDocument();

    expect(screen.getByRole("tab", { name: "All (3)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Installed (2)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Available (0)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reset" })).toBeDisabled();
  });

  it("filters by tab and search", () => {
    renderAt("/org/acme/ui-modules");
    fireEvent.click(screen.getByRole("tab", { name: "Installed (2)" }));
    expect(screen.queryByRole("heading", { name: "Agent Identity" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "AI Gateway" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "Available (0)" }));
    expect(screen.getByText("No modules match.")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "All (3)" }));
    fireEvent.change(screen.getByPlaceholderText("Search modules"), {
      target: { value: "  SECURITY " },
    });
    // Matches the listing category, not just the name.
    expect(screen.getAllByRole("heading", { level: 6 }).map((h) => h.textContent)).toEqual([
      "Agent Identity",
    ]);
  });

  it("uninstalls after confirmation, reinstalls, and resets", async () => {
    renderAt("/org/acme/ui-modules");
    fireEvent.click(within(card("AI Gateway")).getByRole("button", { name: "Uninstall" }));
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText("Uninstall AI Gateway?")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Uninstall" }));

    await waitFor(() =>
      expect(within(card("AI Gateway")).getByRole("button", { name: "Install" })).toBeInTheDocument(),
    );
    expect(screen.getByRole("tab", { name: "Available (1)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Reset" })).toBeEnabled();

    fireEvent.click(within(card("AI Gateway")).getByRole("button", { name: "Install" }));
    expect(within(card("AI Gateway")).getByRole("button", { name: "Installing" })).toBeDisabled();
    expect(within(card("AI Gateway")).getAllByText("Installing")).toHaveLength(2);
    await waitFor(
      () =>
        expect(
          within(card("AI Gateway")).getByRole("button", { name: "Uninstall" }),
        ).toBeInTheDocument(),
      { timeout: 3000 },
    );

    fireEvent.click(screen.getByRole("button", { name: "Reset" }));
    expect(screen.getByRole("button", { name: "Reset" })).toBeDisabled();
  });

  it("opens a module's details page", () => {
    renderAt("/org/acme/ui-modules");
    fireEvent.click(within(card("AI Gateway")).getByRole("button", { name: "Details" }));
    expect(screen.getByRole("heading", { name: "What's included" })).toBeInTheDocument();
  });

  it("shows a checking state while a module's detection runs", () => {
    // Turned on by the operator, so only detection decides.
    runtimeConfig.uiModules = { enable: ["third-party"] };
    renderAt("/org/acme/ui-modules", [
      {
        id: "third-party",
        name: "Third Party",
        description: "Someone else's module",
        apiVersion: PLUGIN_API_VERSION,
        installation: { steps: [], useDetect: () => undefined },
        plugins: [{ id: "tp", name: "TP", apiVersion: PLUGIN_API_VERSION, contributions: [] }],
      },
    ]);
    const tp = card("Third Party");
    expect(within(tp).getByRole("button", { name: "Checking" })).toBeDisabled();
    expect(within(tp).getAllByText("Checking")).toHaveLength(2);
    expect(within(tp).getByText(/by Third party · v1.0.0/)).toBeInTheDocument();
    expect(within(tp).getByText("Extension")).toBeInTheDocument();
    // Still checking counts as available, not installed.
    expect(screen.getByRole("tab", { name: "Available (1)" })).toBeInTheDocument();
  });
});

describe("ModuleSetup", () => {
  it("shows what the module adds and its backend steps", async () => {
    runtimeConfig.ampChartRegistry = "registry.example.com/charts/";
    runtimeConfig.chartVersion = "1.2.3";
    const { container } = renderAt("/org/acme/ui-modules/ai-gateway");
    expect(screen.getByRole("heading", { name: "AI Gateway" })).toBeInTheDocument();
    expect(screen.getByText("Route LLM traffic")).toBeInTheDocument();
    expect(screen.getByText("1 page, 2 menu items, 1 panel")).toBeInTheDocument();
    expect(screen.getByText("wso2-amp-gateway-extension")).toBeInTheDocument();
    expect(screen.getByText("1. Install the chart")).toBeInTheDocument();
    expect(screen.getByText("Run this on the cluster.")).toBeInTheDocument();
    expect(screen.getByText("2. Verify")).toBeInTheDocument();
    expect(container.textContent).toContain(
      "helm install gw oci://registry.example.com/charts/gw --version 1.2.3",
    );
    expect(screen.getByRole("button", { name: "Uninstall" })).toBeInTheDocument();
    expect(await screen.findByText("module setup content")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Back to AI module manager/ })).toHaveAttribute(
      "href",
      "/org/acme/ui-modules",
    );
  });

  it("fills placeholder values when the chart registry and version are unknown", () => {
    const { container } = renderAt("/org/acme/ui-modules/ai-gateway");
    expect(container.textContent).toMatch(/oci:\/\/\S+\/gw --version \S+/);
    expect(container.textContent).not.toContain("{chartRegistry}");
    expect(container.textContent).not.toContain("{chartVersion}");
  });

  it("omits the backend card for a module with no setup steps", () => {
    renderAt("/org/acme/ui-modules/acp-core");
    expect(screen.getByText("Built in")).toBeInTheDocument();
    expect(screen.queryByText("Backend setup")).not.toBeInTheDocument();
  });

  it("shows the 404 page for an unknown module", () => {
    renderAt("/org/acme/ui-modules/nope");
    expect(screen.getByText("Page Not Found")).toBeInTheDocument();
  });
});

describe("moduleUi helpers", () => {
  const base: UIModuleStatus = {
    id: "m",
    name: "M",
    description: "",
    core: false,
    enabled: true,
    installPhase: "installed",
    installation: undefined,
    SetupContent: null,
    plugins: [],
  };

  it.each([
    [{ core: true }, "built-in"],
    [{ enabled: false }, "turned-off"],
    [{ installPhase: "installing" }, "installing"],
    [{ installPhase: "checking" }, "checking"],
    [{ installPhase: "installed" }, "installed"],
    [{ installPhase: "not-installed" }, "available"],
  ] as const)("maps %o to %s", (patch, state) => {
    expect(marketplaceState({ ...base, ...patch })).toBe(state);
  });

  it("pluralises counts and falls back to a third-party listing", () => {
    expect(countLabel(1, "page")).toBe("1 page");
    expect(countLabel(3, "page")).toBe("3 pages");
    expect(listingFor("ai-gateway").category).toBe("Gateway");
    expect(listingFor("unknown").publisher).toBe("Third party");
  });

  it("is off unless the runtime config turns the marketplace on", () => {
    expect(isMarketplaceEnabled()).toBe(false);
    runtimeConfig.uiModules = { marketplace: true };
    expect(isMarketplaceEnabled()).toBe(true);
  });
});

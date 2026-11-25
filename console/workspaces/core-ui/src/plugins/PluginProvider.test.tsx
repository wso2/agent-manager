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

import { Suspense, useSyncExternalStore } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import {
  PLUGIN_API_VERSION,
  PLUGIN_MOUNT_POINTS,
  type PluginActivationState,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import {
  useAgentTypesState,
  useExternalComponentModules,
  useIsUIModuleInstalled,
  useResourceSummaries,
  type ExternalModule,
} from "@agent-management-platform/views";
import {
  PluginActivators,
  PluginProvider,
  pluginKey,
  useUIModuleManager,
  usePluginNavItemsForLevel,
  usePluginRoutes,
} from "./PluginProvider";


const manifest = vi.hoisted(() => ({
  current: { data: undefined, isError: false, isPending: true } as {
    data?: { pages: string[] };
    isError: boolean;
    isPending: boolean;
  },
}));

vi.mock("@agent-management-platform/api-client", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useUIManifest: () => manifest.current,
}));

// A tiny external store so tests can flip activation answers after render.
function createFlag(initial: PluginActivationState) {
  let value = initial;
  const listeners = new Set<() => void>();
  return {
    set(next: PluginActivationState) {
      value = next;
      act(() => listeners.forEach((l) => l()));
    },
    useValue: () =>
      useSyncExternalStore(
        (l) => {
          listeners.add(l);
          return () => listeners.delete(l);
        },
        () => value,
      ),
  };
}

const Icon = () => <svg />;
const pageLoader = () => Promise.resolve({ default: () => <div>page body</div> });
const panelLoader = (text: string) => () =>
  Promise.resolve({ default: () => <div>{text}</div> });

function coreModule(extra: Partial<UIModuleManifest> = {}): UIModuleManifest {
  return {
    id: "core",
    name: "Core",
    description: "Core pages",
    apiVersion: PLUGIN_API_VERSION,
    core: true,
    plugins: [
      {
        id: "core-pages",
        name: "Core pages",
        apiVersion: PLUGIN_API_VERSION,
        contributions: [
          { type: "route", id: "home", level: "org", index: true, load: pageLoader },
          {
            type: "route",
            id: "things",
            level: "project",
            path: "/org/:orgId/project/:projectId/things/*",
            load: pageLoader,
          },
          { type: "route", id: "rel", level: "project", path: "relative", load: pageLoader },
          { type: "route", id: "stray", level: "project", path: "/elsewhere", load: pageLoader },
          {
            type: "nav-item",
            id: "nav-b",
            level: "org",
            label: "B",
            icon: Icon,
            route: { path: "/org/:orgId/b", wildPath: "/org/:orgId/b/*" },
            order: 2,
          },
          {
            type: "nav-item",
            id: "nav-a",
            level: "org",
            label: "A",
            icon: Icon,
            route: { path: "/org/:orgId/a", wildPath: "/org/:orgId/a/*" },
            order: 1,
          },
        ],
      },
    ],
    ...extra,
  };
}

function gatewayModule(extra: Partial<UIModuleManifest> = {}): UIModuleManifest {
  return {
    id: "gateway",
    name: "Gateway",
    description: "Gateway pages",
    apiVersion: PLUGIN_API_VERSION,
    plugins: [
      {
        id: "gw-pages",
        name: "Gateway pages",
        apiVersion: PLUGIN_API_VERSION,
        contributions: [
          { type: "route", id: "list", level: "org", path: "gateways", load: pageLoader },
          {
            type: "nav-item",
            id: "nav",
            level: "org",
            label: "Gateways",
            icon: Icon,
            route: { path: "/org/:orgId/gateways", wildPath: "/org/:orgId/gateways/*" },
          },
          {
            type: "panel",
            id: "panel",
            mountPoint: "top-right",
            load: panelLoader("gateway panel"),
          },
        ],
      },
      {
        id: "gw-extra",
        name: "Gateway extras",
        apiVersion: PLUGIN_API_VERSION,
        contributions: [
          { type: "route", id: "extra", level: "org", path: "extras", load: pageLoader },
        ],
      },
    ],
    ...extra,
  };
}

// JSON drops undefined, so name the pending state.
const stateOf = (state: PluginActivationState) => (state === undefined ? "pending" : state);

/** Renders what the consumer hooks see as JSON, without lazy components. */
function Snapshot({ withPanels = false }: { withPanels?: boolean }) {
  const org = usePluginRoutes("org");
  const project = usePluginRoutes("project");
  const nav = usePluginNavItemsForLevel("org");
  const manager = useUIModuleManager();
  const panels = useExternalComponentModules("top-right");
  const gatewayInstalled = useIsUIModuleInstalled("gateway");
  const agentTypes = useAgentTypesState();
  const summaries = useResourceSummaries();
  const data = {
    org: org.map((r) => ({ key: r.key, path: r.path, index: r.index, state: stateOf(r.state) })),
    project: project.map((r) => ({ key: r.key, path: r.path, state: stateOf(r.state) })),
    nav: nav.map((n) => n.label),
    modules: manager.modules.map((m) => ({
      id: m.id,
      enabled: m.enabled,
      phase: m.installPhase,
      core: m.core,
      plugins: m.plugins,
      hasSetup: !!m.SetupContent,
    })),
    hasChanges: manager.hasChanges,
    panels: panels.map((p) => p.moduleName),
    gatewayInstalled: gatewayInstalled ?? "pending",
    agentTypes: agentTypes.agentTypes.map((t) => t.key),
    agentTypesPending: agentTypes.isPending,
    summaries: summaries.map((s) => s.key),
  };
  return (
    <>
      <pre data-testid="snapshot">{JSON.stringify(data)}</pre>
      {withPanels &&
        panels.map((p) => (
          <Suspense key={p.moduleName}>
            <p.component />
          </Suspense>
        ))}
      <button onClick={() => manager.setInstallPhase("gateway", "not-installed")}>
        uninstall
      </button>
      <button onClick={() => manager.setInstallPhase("gateway", "installing")}>install</button>
      <button onClick={manager.resetChanges}>reset</button>
    </>
  );
}

function snapshot() {
  return JSON.parse(screen.getByTestId("snapshot").textContent ?? "{}");
}

function Host({
  modules,
  externalModules,
  withPanels,
}: {
  modules: UIModuleManifest[];
  externalModules?: ExternalModule[];
  withPanels?: boolean;
}) {
  return (
    <MemoryRouter>
      <PluginProvider modules={modules} externalModules={externalModules}>
        <PluginActivators />
        <Snapshot withPanels={withPanels} />
      </PluginProvider>
    </MemoryRouter>
  );
}

const runtimeConfig = (window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> })
  .__RUNTIME_CONFIG__;

let warn: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  manifest.current = { data: { pages: [] }, isError: false, isPending: false };
  delete runtimeConfig.uiModules;
  warn = vi.spyOn(console, "warn").mockImplementation(() => {});
});

afterEach(() => {
  warn.mockRestore();
});

describe("PluginProvider catalog", () => {
  it("skips modules and plugins built for another plugin API version or with duplicate ids", () => {
    const core = coreModule();
    const modules: UIModuleManifest[] = [
      core,
      { ...gatewayModule(), id: "old", apiVersion: PLUGIN_API_VERSION + 1 },
      { ...coreModule(), name: "Duplicate core" },
      {
        id: "mixed",
        name: "Mixed",
        description: "",
        apiVersion: PLUGIN_API_VERSION,
        core: true,
        plugins: [
          { id: "new-api", name: "N", apiVersion: PLUGIN_API_VERSION + 1, contributions: [] },
          { id: "core-pages", name: "Dup", apiVersion: PLUGIN_API_VERSION, contributions: [] },
          { id: "ok", name: "OK", apiVersion: PLUGIN_API_VERSION, contributions: [] },
        ],
      },
    ];
    render(<Host modules={modules} />);
    const { modules: listed } = snapshot();
    expect(listed.map((m: { id: string }) => m.id)).toEqual(["core", "mixed"]);
    expect(listed[0].plugins).toEqual([
      { id: "core-pages", name: "Core pages", pages: 4, navItems: 2, panels: 0 },
    ]);
    expect(listed[1].plugins.map((p: { id: string }) => p.id)).toEqual(["ok"]);
    const messages = warn.mock.calls.map((c) => String(c[0]));
    expect(messages).toEqual(
      expect.arrayContaining([
        expect.stringContaining('Module "old" targets plugin API'),
        expect.stringContaining('Duplicate module id "core"'),
        expect.stringContaining('Plugin "mixed/new-api" targets plugin API'),
        expect.stringContaining('Duplicate plugin id "core-pages" in "mixed"'),
      ]),
    );
  });
});

describe("routes and nav items", () => {
  it("makes absolute route paths relative to their level and skips paths outside it", () => {
    render(<Host modules={[coreModule()]} />);
    const snap = snapshot();
    expect(snap.org).toEqual([{ key: "core-pages:home", index: true, state: true }]);
    expect(snap.project).toEqual([
      { key: "core-pages:things", path: "things/*", state: true },
      { key: "core-pages:rel", path: "relative", state: true },
    ]);
    expect(warn).toHaveBeenCalledWith(
      expect.stringContaining('Route "core-pages:stray" has path "/elsewhere"'),
    );
  });

  it("sorts nav items by order", () => {
    render(<Host modules={[coreModule()]} />);
    expect(snapshot().nav).toEqual(["A", "B"]);
  });

  it("returns the same lazy component for a route on every render", () => {
    const seen: unknown[] = [];
    function Capture({ tick }: { tick: number }) {
      seen.push(usePluginRoutes("org")[0]?.Component);
      return <span>{tick}</span>;
    }
    const modules = [coreModule()];
    const tree = (tick: number) => (
      <MemoryRouter>
        <PluginProvider modules={modules}>
          <Capture tick={tick} />
        </PluginProvider>
      </MemoryRouter>
    );
    const { rerender } = render(tree(1));
    rerender(tree(2));
    expect(seen.length).toBeGreaterThan(1);
    expect(new Set(seen).size).toBe(1);
  });
});

describe("server UI manifest", () => {
  it("holds non-core modules pending until the manifest loads", () => {
    manifest.current = { data: undefined, isError: false, isPending: true };
    const { rerender } = render(<Host modules={[coreModule(), gatewayModule()]} />);
    let snap = snapshot();
    // Routes still answer deep links (pending), nav items stay hidden.
    expect(snap.org).toContainEqual({ key: "gw-pages:list", path: "gateways", state: "pending" });
    expect(snap.nav).toEqual(["A", "B"]);
    expect(snap.modules.find((m: { id: string }) => m.id === "gateway").phase).toBe("checking");
    expect(snap.gatewayInstalled).toBe("pending");

    manifest.current = {
      data: { pages: [pluginKey("gateway", "gw-pages")] },
      isError: false,
      isPending: false,
    };
    rerender(<Host modules={[coreModule(), gatewayModule()]} />);
    snap = snapshot();
    expect(snap.org).toContainEqual({ key: "gw-pages:list", path: "gateways", state: true });
    // gw-extra is not listed, so its page is off.
    expect(snap.org.map((r: { key: string }) => r.key)).not.toContain("gw-extra:extra");
    // No order means 0, which sorts before A (1) and B (2).
    expect(snap.nav).toEqual(["Gateways", "A", "B"]);
    expect(snap.gatewayInstalled).toBe(true);
  });

  it("turns off a non-core module none of whose pages the manifest lists", () => {
    render(<Host modules={[coreModule(), gatewayModule()]} />);
    const snap = snapshot();
    expect(snap.org.map((r: { key: string }) => r.key)).toEqual(["core-pages:home"]);
    expect(snap.nav).toEqual(["A", "B"]);
    expect(snap.panels).toEqual([]);
    expect(snap.gatewayInstalled).toBe(false);
    const gateway = snap.modules.find((m: { id: string }) => m.id === "gateway");
    expect(gateway.enabled).toBe(false);
    // Core modules stay on whatever the manifest says.
    expect(snap.modules.find((m: { id: string }) => m.id === "core").enabled).toBe(true);
  });

  it("shows only core pages when the manifest cannot be read", () => {
    manifest.current = { data: undefined, isError: true, isPending: false };
    render(<Host modules={[coreModule(), gatewayModule()]} />);
    expect(snapshot().nav).toEqual(["A", "B"]);
    expect(snapshot().gatewayInstalled).toBe(false);
    expect(warn).toHaveBeenCalledWith(expect.stringContaining("Could not read the UI manifest"));
  });

  it("lets the operator config turn on a whole module", () => {
    runtimeConfig.uiModules = { enable: ["gateway"] };
    render(<Host modules={[coreModule(), gatewayModule()]} />);
    const keys = snapshot().org.map((r: { key: string }) => r.key);
    expect(keys).toEqual(["core-pages:home", "gw-pages:list", "gw-extra:extra"]);
  });

  it("lets the operator config turn on a single page", () => {
    runtimeConfig.uiModules = { enable: [pluginKey("gateway", "gw-extra")] };
    render(<Host modules={[coreModule(), gatewayModule()]} />);
    const keys = snapshot().org.map((r: { key: string }) => r.key);
    expect(keys).toEqual(["core-pages:home", "gw-extra:extra"]);
  });
});

describe("activation hooks", () => {
  it("hides a plugin's contributions while pending and when its hook says no", () => {
    const flag = createFlag(undefined);
    const core = coreModule();
    core.plugins[0].useActivation = flag.useValue;
    render(<Host modules={[core]} />);
    expect(snapshot().nav).toEqual([]);
    expect(snapshot().org).toEqual([{ key: "core-pages:home", index: true, state: "pending" }]);

    flag.set(true);
    expect(snapshot().nav).toEqual(["A", "B"]);
    expect(snapshot().org[0].state).toBe(true);

    flag.set(false);
    expect(snapshot().nav).toEqual([]);
    expect(snapshot().org).toEqual([]);
  });

  it("applies a contribution's own hook on top of the plugin's", () => {
    const flag = createFlag(false);
    const core = coreModule();
    const navA = core.plugins[0].contributions.find((c) => c.id === "nav-a")!;
    navA.useActivation = flag.useValue;
    render(<Host modules={[core]} />);
    expect(snapshot().nav).toEqual(["B"]);
    flag.set(true);
    expect(snapshot().nav).toEqual(["A", "B"]);
  });

  it("treats a throwing hook as inactive without breaking the rest", async () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    const core = coreModule();
    core.plugins[0].contributions.find((c) => c.id === "nav-a")!.useActivation = () => {
      throw new Error("boom");
    };
    render(<Host modules={[core]} />);
    await waitFor(() => expect(snapshot().nav).toEqual(["B"]));
    expect(error).toHaveBeenCalledWith(
      expect.stringContaining('Activation hook "core-pages:nav-a" threw'),
      expect.any(Error),
    );
    error.mockRestore();
  });
});

describe("install detection and the module manager", () => {
  it("runs a module's pages only once detection says it is installed", () => {
    const detect = createFlag(undefined);
    runtimeConfig.uiModules = { enable: ["gateway"] };
    const gateway = gatewayModule({
      installation: {
        steps: [],
        useDetect: () => {
          const v = detect.useValue();
          return v === undefined ? undefined : v ? "installed" : "not-installed";
        },
      },
      setup: pageLoader,
    });
    render(<Host modules={[coreModule(), gateway]} />);
    let snap = snapshot();
    const phaseOf = (s: typeof snap) =>
      s.modules.find((m: { id: string }) => m.id === "gateway").phase;
    expect(phaseOf(snap)).toBe("checking");
    expect(snap.modules.find((m: { id: string }) => m.id === "gateway").hasSetup).toBe(true);
    // Still checking: pages run (no flicker) but the agent-type page is told to wait.
    expect(snap.nav).toContain("Gateways");

    detect.set(false);
    snap = snapshot();
    expect(phaseOf(snap)).toBe("not-installed");
    expect(snap.nav).not.toContain("Gateways");
    expect(snap.gatewayInstalled).toBe(false);

    detect.set(true);
    snap = snapshot();
    expect(phaseOf(snap)).toBe("installed");
    expect(snap.nav).toContain("Gateways");
  });

  it("overrides the install phase in memory and resets it", () => {
    runtimeConfig.uiModules = { enable: ["gateway"] };
    render(<Host modules={[coreModule(), gatewayModule()]} />);
    const phase = () =>
      snapshot().modules.find((m: { id: string }) => m.id === "gateway").phase;
    expect(phase()).toBe("installed");
    expect(snapshot().hasChanges).toBe(false);

    act(() => screen.getByText("uninstall").click());
    expect(phase()).toBe("not-installed");
    expect(snapshot().hasChanges).toBe(true);
    expect(snapshot().nav).not.toContain("Gateways");

    act(() => screen.getByText("install").click());
    expect(phase()).toBe("installing");
    expect(snapshot().gatewayInstalled).toBe(false);

    act(() => screen.getByText("reset").click());
    expect(phase()).toBe("installed");
    expect(snapshot().hasChanges).toBe(false);
    expect(snapshot().nav).toContain("Gateways");
  });
});

describe("panels, agent types and resource summaries", () => {
  it("adds active panels after the legacy modules, ordered, and renders them lazily", async () => {
    runtimeConfig.uiModules = { enable: ["gateway"] };
    const gateway = gatewayModule();
    gateway.plugins[0].contributions.push({
      type: "panel",
      id: "first",
      mountPoint: "top-right",
      order: -1,
      load: panelLoader("first panel"),
    });
    const legacy: ExternalModule = {
      kind: "component",
      mountPoint: "top-right",
      moduleName: "legacy",
      component: () => <div>legacy panel</div>,
    };
    render(<Host modules={[coreModule(), gateway]} externalModules={[legacy]} withPanels />);
    expect(snapshot().panels).toEqual(["legacy", "gw-pages:first", "gw-pages:panel"]);
    expect(await screen.findByText("gateway panel")).toBeInTheDocument();
    expect(screen.getByText("first panel")).toBeInTheDocument();
    expect(screen.getByText("legacy panel")).toBeInTheDocument();
  });

  it("lists settled agent types and resource summaries in order", () => {
    const flag = createFlag(undefined);
    const core = coreModule();
    core.plugins[0].contributions.push(
      {
        type: "agent-type",
        id: "external",
        title: "External",
        description: "",
        ctaLabel: "Register",
        illustration: Icon,
        route: { path: "/x", wildPath: "/x/*" },
        order: 2,
      },
      {
        type: "agent-type",
        id: "hosted",
        title: "Hosted",
        description: "",
        ctaLabel: "Create",
        illustration: Icon,
        route: { path: "/y", wildPath: "/y/*" },
        order: 1,
      },
      {
        type: "panel",
        id: "picker",
        mountPoint: PLUGIN_MOUNT_POINTS.addNewAgentOptions,
        load: pageLoader,
        useActivation: flag.useValue,
      },
      {
        type: "resource-summary",
        id: "s2",
        title: "Two",
        description: "",
        icon: Icon,
        order: 2,
        useSummary: () => ({ actionLabel: "", to: "/" }),
      },
      {
        type: "resource-summary",
        id: "s1",
        title: "One",
        description: "",
        icon: Icon,
        order: 1,
        useSummary: () => ({ actionLabel: "", to: "/" }),
      },
    );
    render(<Host modules={[core]} />);
    // The picker panel has not settled, so the page must wait.
    expect(snapshot().agentTypesPending).toBe(true);
    expect(snapshot().agentTypes).toEqual(["core-pages:hosted", "core-pages:external"]);
    expect(snapshot().summaries).toEqual(["core-pages:s1", "core-pages:s2"]);

    flag.set(false);
    expect(snapshot().agentTypesPending).toBe(false);
  });
});

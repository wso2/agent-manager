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

// Plugin host.
//
// Two halves:
// - PluginProvider (in GlobalProviders) knows which plugins are installed and
//   not disabled by config, and holds the latest activation state of each.
// - PluginActivators (inside the signed-in layout route) runs each plugin's
//   activation hook in its own tiny component and reports the result back.
//
// Hooks run in their own components, not in a loop, so every plugin's hook has
// a stable call order, can use any other hook (router params, API queries,
// auth, feature management), and is isolated by an error boundary.

import {
  Component,
  createContext,
  lazy,
  Suspense,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ComponentType,
  type LazyExoticComponent,
  type ReactNode,
} from "react";
import {
  absoluteRouteMap,
  getDisabledUIModules,
  PLUGIN_API_VERSION,
  type PluginActivationHook,
  type PluginActivationState,
  type PluginComponentLoader,
  type PluginLevel,
  type PluginManifest,
  type PluginNavItemContribution,
  type PluginPanelContribution,
  type UIModuleInstallPhase,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import {
  ExternalModuleProvider,
  UIModuleStatusProvider,
  type ExternalComponentModule,
  type ExternalModule,
  type UIModuleStatusMap,
} from "@agent-management-platform/views";
import { useLocation } from "react-router-dom";

export interface ActivePluginRoute {
  key: string;
  level: PluginLevel;
  path?: string;
  index?: boolean;
  /** `undefined` while the plugin's activation hooks are still deciding. */
  state: PluginActivationState;
  Component: LazyExoticComponent<ComponentType>;
}

export interface ActivePluginNavItem extends PluginNavItemContribution {
  key: string;
  pluginId: string;
}

interface ActivationProbeSpec {
  key: string;
  useActivation: PluginActivationHook;
}

interface CatalogEntry {
  module: UIModuleManifest;
  plugins: PluginManifest[];
}

interface PluginHostValue {
  /** Valid modules and their valid plugins, enabled or not. */
  catalog: CatalogEntry[];
  /** Plugins that are enabled by config, in installed modules, and therefore run. */
  plugins: PluginManifest[];
  probes: ActivationProbeSpec[];
  states: Record<string, PluginActivationState>;
  report: (key: string, state: PluginActivationState) => void;
  /** Each module's install phase (detection plus setup-page changes). */
  phases: Record<string, UIModuleInstallPhase>;
  /** Install phases set on setup pages; in memory only for now. */
  installOverrides: Record<string, UIModuleInstallPhase>;
  setInstallPhase: (moduleId: string, phase: UIModuleInstallPhase | undefined) => void;
}

const PluginHostContext = createContext<PluginHostValue>({
  catalog: [],
  plugins: [],
  probes: [],
  states: {},
  report: () => {},
  phases: {},
  installOverrides: {},
  setInstallPhase: () => {},
});

// lazy() must return the same component on every render, or React remounts the
// page each time. Cache per loader function: manifests are module constants, so
// the loader is a stable identity.
const lazyCache = new WeakMap<
  PluginComponentLoader,
  LazyExoticComponent<ComponentType>
>();
function lazyFor(load: PluginComponentLoader) {
  let component = lazyCache.get(load);
  if (!component) {
    component = lazy(load);
    lazyCache.set(load, component);
  }
  return component;
}

const panelCache = new WeakMap<PluginPanelContribution, ExternalComponentModule["component"]>();
function panelComponentFor(panel: PluginPanelContribution) {
  let component = panelCache.get(panel);
  if (!component) {
    const Lazy = lazyFor(panel.load) as ComponentType<Record<string, unknown>>;
    // Panels sit inside existing layouts, so they render nothing while loading
    // rather than a full-page spinner.
    component = function PluginPanel(props: Record<string, unknown>) {
      return (
        <Suspense fallback={null}>
          <Lazy {...props} />
        </Suspense>
      );
    };
    panelCache.set(panel, component);
  }
  return component;
}

function warn(message: string) {
  // eslint-disable-next-line no-console -- surface bad modules to the operator
  console.warn(`[ui-modules] ${message}`);
}

/** Drops modules and plugins that cannot be used, and says why on the console. */
function buildCatalog(modules: UIModuleManifest[]): CatalogEntry[] {
  const seenModules = new Set<string>();
  const seenPlugins = new Set<string>();
  const catalog: CatalogEntry[] = [];
  for (const module of modules) {
    if (module.apiVersion !== PLUGIN_API_VERSION) {
      warn(
        `Module "${module.id}" targets plugin API ${module.apiVersion}, ` +
          `the console supports ${PLUGIN_API_VERSION}; skipping it.`,
      );
      continue;
    }
    if (seenModules.has(module.id)) {
      warn(`Duplicate module id "${module.id}"; skipping the later one.`);
      continue;
    }
    seenModules.add(module.id);
    const plugins = module.plugins.filter((plugin) => {
      if (plugin.apiVersion !== PLUGIN_API_VERSION) {
        warn(
          `Plugin "${module.id}/${plugin.id}" targets plugin API ${plugin.apiVersion}, ` +
            `the console supports ${PLUGIN_API_VERSION}; skipping it.`,
        );
        return false;
      }
      if (seenPlugins.has(plugin.id)) {
        warn(`Duplicate plugin id "${plugin.id}" in "${module.id}"; skipping it.`);
        return false;
      }
      seenPlugins.add(plugin.id);
      return true;
    });
    catalog.push({ module, plugins });
  }
  return catalog;
}

/** Key for a plugin in the disabled list: "<module>/<plugin>". */
export const pluginKey = (moduleId: string, pluginId: string) => `${moduleId}/${pluginId}`;

/** What public/ui-modules.js turns off: module ids, "<module>/<plugin>" or plugin ids. */
function configDisabled(): Set<string> {
  return new Set(getDisabledUIModules());
}

function moduleEnabledByConfig(disabled: Set<string>, moduleId: string) {
  return !disabled.has(moduleId);
}

function pluginEnabledByConfig(disabled: Set<string>, moduleId: string, pluginId: string) {
  return !disabled.has(pluginKey(moduleId, pluginId)) && !disabled.has(pluginId);
}

/** Probe key for a module's install detection hook. */
const installKey = (moduleId: string) => `install:${moduleId}`;

function installPhaseOf(
  module: UIModuleManifest,
  states: Record<string, PluginActivationState>,
  installOverrides: Record<string, UIModuleInstallPhase>,
): UIModuleInstallPhase {
  if (module.core) return "installed";
  const override = installOverrides[module.id];
  if (override) return override;
  if (!module.installation?.useDetect) return "installed";
  const detected = states[installKey(module.id)];
  if (detected === undefined) return "checking";
  return detected ? "installed" : "not-installed";
}

/** Pages run while a module is installed, or still being checked (no flicker). */
const pagesMayRun = (phase: UIModuleInstallPhase) =>
  phase === "installed" || phase === "checking";

const contributionKey = (pluginId: string, contributionId: string) =>
  `${pluginId}:${contributionId}`;

/**
 * Combines the plugin's and the contribution's activation. Off wins over
 * pending, pending wins over on. A missing hook counts as on.
 */
function combine(
  states: Record<string, PluginActivationState>,
  ...keys: Array<string | null>
): PluginActivationState {
  let pending = false;
  for (const key of keys) {
    if (key === null) continue;
    const state = states[key];
    if (state === false) return false;
    if (state === undefined) pending = true;
  }
  return pending ? undefined : true;
}

interface ActiveContribution<T> {
  plugin: PluginManifest;
  contribution: T;
  key: string;
  state: PluginActivationState;
}

/** Every contribution of the usable plugins with its combined activation state. */
function useContributions() {
  const { plugins, states } = useContext(PluginHostContext);
  return useMemo(() => {
    const result: ActiveContribution<PluginManifest["contributions"][number]>[] = [];
    for (const plugin of plugins) {
      for (const contribution of plugin.contributions) {
        const key = contributionKey(plugin.id, contribution.id);
        const state = combine(
          states,
          plugin.useActivation ? plugin.id : null,
          contribution.useActivation ? key : null,
        );
        result.push({ plugin, contribution, key, state });
      }
    }
    return result;
  }, [plugins, states]);
}

export interface PluginProviderProps {
  modules: UIModuleManifest[];
  /** Modules passed to CoreUI the legacy way; active plugin panels are added. */
  externalModules?: ExternalModule[];
  children: ReactNode;
}

const NO_MODULES: ExternalModule[] = [];

export function PluginProvider({
  modules,
  externalModules = NO_MODULES,
  children,
}: PluginProviderProps) {
  const catalog = useMemo(() => buildCatalog(modules), [modules]);

  const [installOverrides, setInstallOverrides] = useState<
    Record<string, UIModuleInstallPhase>
  >({});
  const setInstallPhase = useCallback(
    (moduleId: string, phase: UIModuleInstallPhase | undefined) => {
      setInstallOverrides((prev) => {
        const next = { ...prev };
        if (phase === undefined) delete next[moduleId];
        else next[moduleId] = phase;
        return next;
      });
    },
    [],
  );

  const [states, setStates] = useState<Record<string, PluginActivationState>>({});
  const report = useCallback((key: string, state: PluginActivationState) => {
    setStates((prev) =>
      key in prev && prev[key] === state ? prev : { ...prev, [key]: state },
    );
  }, []);

  // Install detection runs for every module, enabled or not, so the Module
  // manager can show what is installed.
  const installProbes = useMemo(() => {
    const list: ActivationProbeSpec[] = [];
    for (const { module } of catalog) {
      const useDetect = module.installation?.useDetect;
      if (module.core || !useDetect) continue;
      list.push({
        key: installKey(module.id),
        useActivation: () => {
          const detected = useDetect();
          return detected === undefined ? undefined : detected === "installed";
        },
      });
    }
    return list;
  }, [catalog]);

  const phases = useMemo(() => {
    const result: Record<string, UIModuleInstallPhase> = {};
    for (const { module } of catalog) {
      result[module.id] = installPhaseOf(module, states, installOverrides);
    }
    return result;
  }, [catalog, states, installOverrides]);

  // Install status for pages in other modules (useIsUIModuleInstalled). A module
  // the runtime config turns off counts as not installed.
  const moduleStatus = useMemo<UIModuleStatusMap>(() => {
    const disabled = configDisabled();
    const result: UIModuleStatusMap = {};
    for (const { module } of catalog) {
      const enabled = !!module.core || moduleEnabledByConfig(disabled, module.id);
      result[module.id] = enabled ? phases[module.id] ?? "installed" : "not-installed";
    }
    return result;
  }, [catalog, phases]);

  // What the runtime config turns off stays off for the page's lifetime; the
  // AI module manager cannot switch it back on.
  const usable = useMemo(() => {
    const disabled = configDisabled();
    const list: PluginManifest[] = [];
    for (const { module, plugins } of catalog) {
      if (!pagesMayRun(phases[module.id] ?? "installed")) continue;
      // Core modules cannot be turned off as a whole.
      if (!module.core && !moduleEnabledByConfig(disabled, module.id)) continue;
      for (const plugin of plugins) {
        if (pluginEnabledByConfig(disabled, module.id, plugin.id)) list.push(plugin);
      }
    }
    return list;
  }, [catalog, phases]);

  const probes = useMemo(() => {
    const list: ActivationProbeSpec[] = [...installProbes];
    for (const plugin of usable) {
      if (plugin.useActivation) {
        list.push({ key: plugin.id, useActivation: plugin.useActivation });
      }
      for (const contribution of plugin.contributions) {
        if (contribution.useActivation) {
          list.push({
            key: contributionKey(plugin.id, contribution.id),
            useActivation: contribution.useActivation,
          });
        }
      }
    }
    return list;
  }, [usable, installProbes]);

  // Forget the answers of probes that no longer run (module uninstalled or
  // turned off), so a plugin that comes back starts as pending instead of
  // reusing an answer given on another page.
  useEffect(() => {
    const live = new Set(probes.map((probe) => probe.key));
    setStates((prev) => {
      const stale = Object.keys(prev).filter((key) => !live.has(key));
      if (!stale.length) return prev;
      const next = { ...prev };
      for (const key of stale) delete next[key];
      return next;
    });
  }, [probes]);

  const value = useMemo(
    () => ({
      catalog,
      plugins: usable,
      probes,
      states,
      report,
      phases,
      installOverrides,
      setInstallPhase,
    }),
    [
      catalog,
      usable,
      probes,
      states,
      report,
      phases,
      installOverrides,
      setInstallPhase,
    ],
  );

  return (
    <PluginHostContext.Provider value={value}>
      <UIModuleStatusProvider value={moduleStatus}>
        <PanelModules externalModules={externalModules}>{children}</PanelModules>
      </UIModuleStatusProvider>
    </PluginHostContext.Provider>
  );
}

/** Feeds active plugin panels into the existing mount-point system. */
function PanelModules({
  externalModules,
  children,
}: {
  externalModules: ExternalModule[];
  children: ReactNode;
}) {
  const contributions = useContributions();
  const modules = useMemo(() => {
    const panels: ExternalComponentModule[] = [];
    for (const { contribution, key, state } of contributions) {
      if (contribution.type === "panel" && state === true) {
        panels.push({
          kind: "component",
          mountPoint: contribution.mountPoint,
          moduleName: key,
          component: panelComponentFor(contribution),
        });
      }
    }
    return panels.length ? [...externalModules, ...panels] : externalModules;
  }, [contributions, externalModules]);
  return (
    <ExternalModuleProvider externalPageModules={modules}>
      {children}
    </ExternalModuleProvider>
  );
}

// ---------------------------------------------------------------------------
// Activation

function ActivationProbe({
  probeKey,
  useActivation,
  report,
}: {
  probeKey: string;
  useActivation: PluginActivationHook;
  report: PluginHostValue["report"];
}) {
  const state = useActivation();
  useEffect(() => {
    report(probeKey, state);
  }, [probeKey, state, report]);
  return null;
}

class ActivationBoundary extends Component<
  {
    probeKey: string;
    report: PluginHostValue["report"];
    /** Changes on navigation, so a hook that threw gets another try on the next page. */
    resetKey: string;
    children: ReactNode;
  },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidUpdate(prev: { resetKey: string }) {
    if (this.state.failed && prev.resetKey !== this.props.resetKey) {
      this.setState({ failed: false });
    }
  }

  componentDidCatch(error: unknown) {
    // A broken plugin must not take the console down with it.
    // eslint-disable-next-line no-console -- surface bad plugins to the operator
    console.error(
      `[plugins] Activation hook "${this.props.probeKey}" threw; treating it as inactive.`,
      error,
    );
    this.props.report(this.props.probeKey, false);
  }

  render() {
    return this.state.failed ? null : this.props.children;
  }
}

/**
 * Runs every plugin activation hook. Render it once inside the router and the
 * signed-in providers, at a route that matches all console pages, so hooks can
 * read the current org/project/agent params and call authenticated APIs.
 */
export function PluginActivators() {
  const { probes, report } = useContext(PluginHostContext);
  const { pathname } = useLocation();
  return (
    <>
      {probes.map(({ key, useActivation }) => (
        // Keyed by id: a plugin keeps its probe (and hook state) across pages.
        <ActivationBoundary key={key} probeKey={key} report={report} resetKey={pathname}>
          <ActivationProbe
            probeKey={key}
            useActivation={useActivation}
            report={report}
          />
        </ActivationBoundary>
      ))}
    </>
  );
}

// ---------------------------------------------------------------------------
// Consumers

/**
 * Plugin pages for one level. Every enabled plugin's routes are returned,
 * including pending ones, so a deep link does not 404 while activation hooks
 * are still loading their data; render them with PluginRouteElement.
 */
/** Absolute route of each level, which absolute plugin paths must start with. */
const LEVEL_PATHS: Record<PluginLevel, string> = {
  org: absoluteRouteMap.children.org.path,
  project: absoluteRouteMap.children.org.children.projects.path,
  agent: absoluteRouteMap.children.org.children.projects.children.agents.path,
};

/**
 * Turns a plugin route path into one relative to its level. Relative paths
 * pass through; an absolute path (an absoluteRouteMap entry, optionally with
 * "/*") has the level's own path stripped. Returns null when an absolute path
 * is not under the level, which React Router would otherwise throw on,
 * taking the whole console down.
 */
function relativeToLevel(level: PluginLevel, path: string, key: string): string | null {
  if (!path.startsWith("/")) return path;
  const base = LEVEL_PATHS[level];
  if (path.startsWith(base + "/")) return path.slice(base.length + 1);
  warn(`Route "${key}" has path "${path}", which is not under its ${level} level (${base}); skipping it.`);
  return null;
}

export function usePluginRoutes(level: PluginLevel): ActivePluginRoute[] {
  const contributions = useContributions();
  return useMemo(() => {
    const routes: ActivePluginRoute[] = [];
    for (const { contribution, key, state } of contributions) {
      if (contribution.type === "route" && contribution.level === level && state !== false) {
        const path =
          contribution.path === undefined
            ? undefined
            : relativeToLevel(level, contribution.path, key);
        if (path === null) continue;
        routes.push({
          key,
          level,
          path,
          index: contribution.index,
          state,
          Component: lazyFor(contribution.load),
        });
      }
    }
    return routes;
  }, [contributions, level]);
}

/**
 * Every active plugin nav item for one level, in any section, sorted by
 * `order`. Pending items are hidden until their activation settles.
 */
export function usePluginNavItemsForLevel(level: PluginLevel): ActivePluginNavItem[] {
  const contributions = useContributions();
  return useMemo(() => {
    const items: ActivePluginNavItem[] = [];
    for (const { plugin, contribution, key, state } of contributions) {
      if (contribution.type === "nav-item" && state === true && contribution.level === level) {
        items.push({ ...contribution, key, pluginId: plugin.id });
      }
    }
    return items.sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
  }, [contributions, level]);
}

// ---------------------------------------------------------------------------
// Module manager

export interface UIModulePluginStatus {
  id: string;
  name: string;
  pages: number;
  navItems: number;
  panels: number;
}

export interface UIModuleStatus {
  id: string;
  name: string;
  description: string;
  /** Core modules are always installed and cannot be turned off as a whole. */
  core: boolean;
  /** False when public/ui-modules.js turns the module off. */
  enabled: boolean;
  /** Where the module is in its install lifecycle. */
  installPhase: UIModuleInstallPhase;
  installation: UIModuleManifest["installation"];
  /** The module's own extra setup content, if any. */
  SetupContent: LazyExoticComponent<ComponentType> | null;
  plugins: UIModulePluginStatus[];
}

/**
 * Every module with its install state, plus a setter for the install phase
 * (kept in memory until the page reloads).
 */
export function useUIModuleManager() {
  const { catalog, phases, installOverrides, setInstallPhase } =
    useContext(PluginHostContext);

  const modules = useMemo<UIModuleStatus[]>(() => {
    const disabled = configDisabled();
    return catalog.map(({ module, plugins }) => ({
      id: module.id,
      name: module.name,
      description: module.description,
      core: !!module.core,
      enabled: !!module.core || moduleEnabledByConfig(disabled, module.id),
      installPhase: phases[module.id] ?? "installed",
      installation: module.installation,
      SetupContent: module.setup ? lazyFor(module.setup) : null,
      plugins: plugins.map((plugin) => {
        const count = (type: string) =>
          plugin.contributions.filter((c) => c.type === type).length;
        return {
          id: plugin.id,
          name: plugin.name,
          pages: count("route"),
          navItems: count("nav-item"),
          panels: count("panel"),
        };
      }),
    }));
  }, [catalog, phases]);

  const resetChanges = useCallback(() => {
    for (const moduleId of Object.keys(installOverrides)) {
      setInstallPhase(moduleId, undefined);
    }
  }, [installOverrides, setInstallPhase]);

  return {
    modules,
    /** True when an install phase was changed here since load. */
    hasChanges: Object.keys(installOverrides).length > 0,
    /** Sets a module's install phase; undefined goes back to detection. */
    setInstallPhase,
    resetChanges,
  };
}

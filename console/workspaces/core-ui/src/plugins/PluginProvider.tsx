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
  globalConfig,
  PLUGIN_API_VERSION,
  type PluginActivationHook,
  type PluginActivationState,
  type PluginComponentLoader,
  type PluginLevel,
  type PluginManifest,
  type PluginNavItemContribution,
  type PluginPanelContribution,
} from "@agent-management-platform/types";
import {
  ExternalModuleProvider,
  type ExternalComponentModule,
  type ExternalModule,
} from "@agent-management-platform/views";

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

interface PluginHostValue {
  /** Installed plugins minus invalid and config-disabled ones. */
  plugins: PluginManifest[];
  probes: ActivationProbeSpec[];
  states: Record<string, PluginActivationState>;
  report: (key: string, state: PluginActivationState) => void;
}

const PluginHostContext = createContext<PluginHostValue>({
  plugins: [],
  probes: [],
  states: {},
  report: () => {},
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

/** Drops manifests that cannot be used, and says why on the console. */
function usableManifests(plugins: PluginManifest[]): PluginManifest[] {
  const disabled = new Set(globalConfig?.plugins?.disabled ?? []);
  const seen = new Set<string>();
  return plugins.filter((plugin) => {
    if (plugin.apiVersion !== PLUGIN_API_VERSION) {
      // eslint-disable-next-line no-console -- surface bad plugins to the operator
      console.warn(
        `[plugins] "${plugin.id}" targets plugin API ${plugin.apiVersion}, ` +
          `the console supports ${PLUGIN_API_VERSION}; skipping it.`,
      );
      return false;
    }
    if (seen.has(plugin.id)) {
      // eslint-disable-next-line no-console -- surface bad plugins to the operator
      console.warn(`[plugins] Duplicate plugin id "${plugin.id}"; skipping the later one.`);
      return false;
    }
    seen.add(plugin.id);
    return !disabled.has(plugin.id) && !(plugin.module && disabled.has(plugin.module));
  });
}

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
  plugins: PluginManifest[];
  /** Modules passed to CoreUI the legacy way; active plugin panels are added. */
  externalModules?: ExternalModule[];
  children: ReactNode;
}

const NO_MODULES: ExternalModule[] = [];

export function PluginProvider({
  plugins,
  externalModules = NO_MODULES,
  children,
}: PluginProviderProps) {
  const usable = useMemo(() => usableManifests(plugins), [plugins]);

  const probes = useMemo(() => {
    const list: ActivationProbeSpec[] = [];
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
  }, [usable]);

  const [states, setStates] = useState<Record<string, PluginActivationState>>({});
  const report = useCallback((key: string, state: PluginActivationState) => {
    setStates((prev) =>
      key in prev && prev[key] === state ? prev : { ...prev, [key]: state },
    );
  }, []);

  const value = useMemo(
    () => ({ plugins: usable, probes, states, report }),
    [usable, probes, states, report],
  );

  return (
    <PluginHostContext.Provider value={value}>
      <PanelModules externalModules={externalModules}>{children}</PanelModules>
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
  { probeKey: string; report: PluginHostValue["report"]; children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
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
  return (
    <>
      {probes.map(({ key, useActivation }) => (
        // Keyed by id: a plugin keeps its probe (and hook state) across pages.
        <ActivationBoundary key={key} probeKey={key} report={report}>
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
export function usePluginRoutes(level: PluginLevel): ActivePluginRoute[] {
  const contributions = useContributions();
  return useMemo(() => {
    const routes: ActivePluginRoute[] = [];
    for (const { contribution, key, state } of contributions) {
      if (contribution.type === "route" && contribution.level === level && state !== false) {
        routes.push({
          key,
          level,
          path: contribution.path,
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

/**
 * Active plugin nav items for one level, sorted by `order`. Pass a section key
 * to get that section's items, or omit it for the items that have no section.
 * Pending items are hidden until their activation settles.
 */
export function usePluginNavItems(
  level: PluginLevel,
  section?: string,
): ActivePluginNavItem[] {
  const contributions = useContributions();
  return useMemo(() => {
    const items: ActivePluginNavItem[] = [];
    for (const { plugin, contribution, key, state } of contributions) {
      if (
        contribution.type === "nav-item" &&
        state === true &&
        contribution.level === level &&
        contribution.section === section
      ) {
        items.push({ ...contribution, key, pluginId: plugin.id });
      }
    }
    return items.sort((a, b) => (a.order ?? 0) - (b.order ?? 0));
  }, [contributions, level, section]);
}

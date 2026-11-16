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

// Plugin contract for console feature modules.
//
// A plugin is described by a small manifest that the shell imports eagerly. The
// manifest holds only metadata (ids, labels, icons, route patterns) and loader
// functions; the page code itself sits behind a dynamic import() in `load`, so a
// plugin that is disabled or not activated never has its chunk requested.
//
// Keep a plugin's manifest in its own module (by convention `src/plugin.ts`,
// exported as `<package>/plugin`) and do not import page components from it, or
// the page code is pulled into the shell's main chunk.

import type { ComponentType } from "react";
import type { IconType } from "../page-metadata";

/** Version of this contract. The shell rejects manifests built for another one. */
export const PLUGIN_API_VERSION = 1;

/** Where a contribution sits in the console's org > project > agent hierarchy. */
export type PluginLevel = "org" | "project" | "agent";

/**
 * Activation result: `true` turns the plugin (or contribution) on, `false`
 * turns it off, and `undefined` means "still deciding" (for example while a
 * query loads). Pending routes show a loader, pending nav items and panels stay
 * hidden.
 */
export type PluginActivationState = boolean | undefined;

/**
 * A React hook, written by the plugin developer, that decides whether the
 * plugin or one of its contributions is active.
 *
 * Because it is a hook it can use anything the console provides: route params
 * (useParams), the API client and its queries (useGetAgent, useTokenScopes,
 * useEventsEnabled, ...), feature management (useFeatureFlag, useGatewayFeatures),
 * auth, and so on. The shell calls it on every page inside the signed-in layout,
 * under the router, so params for the current org/project/agent are available.
 *
 * Rules:
 * - Name it `use...` and follow the rules of hooks.
 * - Return `undefined` while data is loading rather than guessing.
 * - Keep it cheap: it runs on every page. Prefer cached queries.
 * - If it throws, the shell treats it as inactive; it cannot break the console.
 *
 * @example
 *   // Only for agents of type agent-api, once the agent has loaded.
 *   useActivation: () => {
 *     const { orgId, projectId, agentId } = useParams();
 *     const { data: agent, isLoading } = useGetAgent({
 *       orgName: orgId, projName: projectId, agentName: agentId,
 *     });
 *     if (isLoading) return undefined;
 *     return agent?.agentType?.type === "agent-api";
 *   }
 */
export type PluginActivationHook = () => PluginActivationState;

/** Lazily loads a React component, normally through a dynamic import(). */
export type PluginComponentLoader = () => Promise<{ default: ComponentType }>;

interface PluginContributionBase {
  /** Unique within the plugin. */
  id: string;
  /**
   * Activation hook for this contribution only. It applies on top of the
   * plugin's own: the contribution is active when both are. Use it when one
   * plugin ships several pages that are gated differently.
   */
  useActivation?: PluginActivationHook;
}

/** A page mounted under a level's route. */
export interface PluginRouteContribution extends PluginContributionBase {
  type: "route";
  level: PluginLevel;
  /**
   * Where the page is mounted. Either absolute, preferably straight from
   * absoluteRouteMap (e.g. `agents.children.environment.children.observability
   * .children.metrics.path`), or relative to the level's route. Absolute paths
   * must start with the level's own path; the shell makes them relative and
   * skips (with a console warning) one that does not. Append "/*" when the
   * page owns nested routes. Omit it together with `index: true` for the
   * level's landing page (e.g. the org overview).
   */
  path?: string;
  /** The level's index route, rendered at the level's own URL. */
  index?: boolean;
  load: PluginComponentLoader;
}

/** Route pattern pair, as found in absoluteRouteMap entries. */
export interface PluginNavRoute {
  /** Absolute pattern used to build the link, e.g. "/org/:orgId/gateways". */
  path: string;
  /** Pattern matched against the current URL to highlight the item. */
  wildPath: string;
}

/** A left-navigation entry. */
export interface PluginNavItemContribution extends PluginContributionBase {
  type: "nav-item";
  level: PluginLevel;
  label: string;
  icon: IconType;
  route: PluginNavRoute;
  /** Extra URL patterns that also highlight the item (e.g. an edit page). */
  activePaths?: string[];
  /**
   * Shell-defined slot to place the item in. Each level has flat slots ("main"
   * at the top, "footer" after the sections) and collapsible sections:
   * - org: "resources", "evaluation", "infrastructure", "agent-identities"
   * - agent: "lifecycle", "security", "observability", "evaluation"
   * Items without a section are appended after everything else.
   */
  section?: string;
  /** Sort key within the slot; lower comes first. Defaults to 0. */
  order?: number;
  /** Draw a divider above the item. */
  dividerBefore?: boolean;
  /** Pin the item to the bottom of the navigation (e.g. Settings). */
  pinBottom?: boolean;
}

/** A component rendered at one of the shell's mount points (see MountPoints). */
export interface PluginPanelContribution extends PluginContributionBase {
  type: "panel";
  mountPoint: string;
  load: PluginComponentLoader;
}

export type PluginContribution =
  | PluginRouteContribution
  | PluginNavItemContribution
  | PluginPanelContribution;

export interface PluginManifest {
  /** Stable id, used in the runtime config to disable the plugin. */
  id: string;
  /** Human-readable name. */
  name: string;
  /** Must equal PLUGIN_API_VERSION. */
  apiVersion: number;
  /**
   * Plugin-wide activation hook; when it returns false no contribution is
   * active. Omit it to be always on (subject to the config's disabled list).
   */
  useActivation?: PluginActivationHook;
  contributions: PluginContribution[];
}

/**
 * A UI module: a product area of the console (for example "AI Gateway") made
 * of one or more page plugins. Modules are the unit operators turn on and off;
 * each page plugin inside can also be turned off on its own.
 *
 * In the repository a module lives in workspaces/ui-modules/<id>: the folder is
 * a package that exports this manifest, and its pages are packages under
 * workspaces/ui-modules/<id>/pages.
 */
export interface UIModuleManifest {
  /** Stable id, e.g. "ai-gateway". Used in the disabled list. */
  id: string;
  /** Display name, e.g. "AI Gateway". */
  name: string;
  /** One line on what the module provides. */
  description: string;
  /** Must equal PLUGIN_API_VERSION. */
  apiVersion: number;
  /**
   * Core modules ship with every console: always installed, and the module as
   * a whole cannot be turned off (single pages still can).
   */
  core?: boolean;
  /**
   * How the module's backend gets installed. Each module has a setup page
   * (/org/:orgId/ui-modules/<id>) that walks through these steps. Omit for a
   * module that needs nothing beyond the console.
   */
  installation?: UIModuleInstallation;
  /**
   * Extra, module-specific content for the setup page (settings, health
   * checks). Rendered below the install and page sections.
   */
  setup?: PluginComponentLoader;
  /** The module's page plugins. Plugin ids must be unique across modules. */
  plugins: PluginManifest[];
}

/** Whether a module's backend is present. */
export type UIModuleInstallState = "installed" | "not-installed";

/**
 * Where a module is in its install lifecycle, as the console sees it:
 * "checking" while detection runs, "installing" while someone works through
 * the setup page's steps. A module's pages show only when it is installed
 * (or still being checked, to avoid flicker).
 */
export type UIModuleInstallPhase = UIModuleInstallState | "installing" | "checking";

/** One step on a module's setup page. */
export interface UIModuleInstallStep {
  title: string;
  description?: string;
  /**
   * Shell command to run, shown with a copy button. "{chartRegistry}" and
   * "{chartVersion}" are filled from the runtime config (ampChartRegistry,
   * chartVersion).
   */
  command?: string;
}

export interface UIModuleInstallation {
  /** Helm chart that provides the module's backend, e.g. "wso2-amp-observability-extension". */
  chart?: string;
  /** Ordered steps for installing the backend. */
  steps: UIModuleInstallStep[];
  /**
   * React hook, written by the module, that detects whether the backend is
   * installed (for example from the service's /api/v1/config). Return
   * undefined while checking. Without it the module counts as installed until
   * someone marks it otherwise on its setup page.
   */
  useDetect?: () => UIModuleInstallState | undefined;
}

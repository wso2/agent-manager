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

import type { ReactNode } from "react";
import { generatePath, matchPath } from "react-router-dom";
import {
  absoluteRouteMap,
  type AgentResponse,
} from "@agent-management-platform/types";
import { isExternalAgent } from "@agent-management-platform/views";

/**
 * The left menu is the same at every level. Each entry says which levels its
 * page lives at; opening it anywhere else lands on that level's gate page
 * (`go/:navKey`), which asks for the missing project/agent or points upwards.
 */
export type NavLevel = "org" | "project" | "agent";

/** Broadest first. */
export const NAV_LEVELS: readonly NavLevel[] = ["org", "project", "agent"];

export interface NavRoute {
  /** Absolute route pattern, from `absoluteRouteMap`. */
  path: string;
  /** Patterns that mark the entry active. Defaults to the route and everything under it. */
  activePaths?: string[];
}

export type NavSectionId =
  | "lifecycle"
  | "security"
  | "observability"
  | "evaluation"
  | "infrastructure"
  | "identities";

export interface NavEntry {
  /** Stable id, also the `:navKey` of the gate URL. */
  id: string;
  /** Shown in the menu; must be unique, the layout keys the active item by it. */
  label: string;
  icon: ReactNode;
  section?: NavSectionId;
  /** Where the page lives. An entry with no routes is not shown. */
  routes: Partial<Record<NavLevel, NavRoute>>;
  /** Why the given agent can't use this page; undefined when it can. */
  unsupportedReason?: (agent: AgentResponse) => string | undefined;
  /** Who the page is for, as "{label} is only available for …". */
  availableFor?: string;
  pinBottom?: boolean;
  afterSections?: boolean;
  dividerBefore?: boolean;
  /**
   * Shown only at the levels it has a route for, rather than everywhere with a
   * gate. For listings the top bar already switches between (Projects, Agents)
   * and for agent-only sections (lifecycle, security).
   */
  ownLevelOnly?: boolean;
}

/**
 * Whether the entry is listed in the menu at `level`. Given the current agent,
 * own-level entries that agent can't use are left out too.
 */
export function isNavEntryShownAt(
  entry: NavEntry,
  level: NavLevel,
  agent?: AgentResponse,
): boolean {
  if (!entry.ownLevelOnly) return true;
  if (!entry.routes[level]) return false;
  return !(level === "agent" && agent && entry.unsupportedReason?.(agent));
}

export interface NavParams {
  orgId?: string;
  projectId?: string;
  agentId?: string;
  envId?: string;
}

/** Router state the top bar passes to a gate: where the user came from. */
export interface LevelGateFrom {
  projectId?: string;
  agentId?: string;
}

export type NavTarget =
  | { kind: "direct"; level: NavLevel }
  /** The page lives deeper: the user picks the project and/or agent. */
  | { kind: "pick"; level: NavLevel }
  /** The page lives higher: the user is sent back up. */
  | { kind: "up"; level: NavLevel };

export function currentNavLevel(params: NavParams): NavLevel | undefined {
  if (params.orgId && params.projectId && params.agentId) return "agent";
  if (params.orgId && params.projectId) return "project";
  if (params.orgId) return "org";
  return undefined;
}

export function resolveNavTarget(
  entry: NavEntry,
  level: NavLevel,
): NavTarget | undefined {
  if (entry.routes[level]) return { kind: "direct", level };
  const index = NAV_LEVELS.indexOf(level);
  // A higher level needs no picking, so it wins over a deeper one.
  for (let i = index - 1; i >= 0; i--) {
    if (entry.routes[NAV_LEVELS[i]]) return { kind: "up", level: NAV_LEVELS[i] };
  }
  for (let i = index + 1; i < NAV_LEVELS.length; i++) {
    if (entry.routes[NAV_LEVELS[i]]) return { kind: "pick", level: NAV_LEVELS[i] };
  }
  return undefined;
}

export function routeNeedsEnv(route: NavRoute): boolean {
  return route.path.includes(":envId");
}

const GATE_PATHS: Record<NavLevel, string> = {
  org: absoluteRouteMap.children.org.children.levelGate.path,
  project: absoluteRouteMap.children.org.children.projects.children.levelGate.path,
  agent:
    absoluteRouteMap.children.org.children.projects.children.agents.children
      .levelGate.path,
};

export function gatePath(level: NavLevel, params: NavParams, navKey: string) {
  return generatePath(GATE_PATHS[level], { ...params, navKey });
}

const LEVEL_HOME_PATHS: Record<NavLevel, string> = {
  org: absoluteRouteMap.children.org.path,
  project: absoluteRouteMap.children.org.children.projects.path,
  agent: absoluteRouteMap.children.org.children.projects.children.agents.path,
};

/** The level's own home page (its overview). */
export function levelHomePath(level: NavLevel, params: NavParams) {
  return generatePath(LEVEL_HOME_PATHS[level], params);
}

/** The page's own URL at `level`; undefined if it lives elsewhere or still needs an environment. */
export function entryRoutePath(
  entry: NavEntry,
  level: NavLevel,
  params: NavParams,
): string | undefined {
  const route = entry.routes[level];
  if (!route || (routeNeedsEnv(route) && !params.envId)) return undefined;
  return generatePath(route.path, params);
}

/**
 * Where the menu item links to. The page itself when it lives at this level and
 * suits the current agent; the gate page otherwise.
 */
export function navEntryHref(
  entry: NavEntry,
  level: NavLevel,
  params: NavParams,
  agent?: AgentResponse,
): string {
  const unsupported =
    level === "agent" && agent ? entry.unsupportedReason?.(agent) : undefined;
  const direct = unsupported ? undefined : entryRoutePath(entry, level, params);
  return direct ?? gatePath(level, params, entry.id);
}

/**
 * The menu entry the user is on: the one a gate URL names, or the one whose
 * route matches. The gate is matched once, not once per entry.
 */
export function findActiveEntry(
  entries: NavEntry[],
  pathname: string,
): NavEntry | undefined {
  const gateKey = NAV_LEVELS.map(
    (level) => matchPath(GATE_PATHS[level], pathname)?.params.navKey,
  ).find(Boolean);
  if (gateKey) return entries.find((entry) => entry.id === gateKey);
  return entries.find((entry) =>
    Object.values(entry.routes).some((route) =>
      (route?.activePaths ?? [`${route?.path}/*`]).some(
        (pattern) => !!matchPath(pattern, pathname),
      ),
    ),
  );
}

// Which agents an agent-level page applies to. Attached to entries in
// useNavEntries and read by the menu, the gate and AgentGuard alike.
const isFromCatalog = (agent: AgentResponse) =>
  !isExternalAgent(agent) && !!agent.kindName;

const EXTERNAL = "External agents run outside the platform, so this page doesn't apply to them.";
const CATALOG = "Agents created from the catalog are built from their kind, so this page doesn't apply to them.";

type AgentRule = Pick<NavEntry, "unsupportedReason" | "availableFor">;

/** Pages for agents the platform runs (Deploy, Try it, logs, metrics). */
export const PLATFORM_AGENTS_ONLY: AgentRule = {
  unsupportedReason: (agent) => (isExternalAgent(agent) ? EXTERNAL : undefined),
  availableFor: "agents that run on the platform",
};

/** Pages for agents built from their own source (Build, Publish). */
export const SOURCE_AGENTS_ONLY: AgentRule = {
  unsupportedReason: (agent) =>
    isExternalAgent(agent) ? EXTERNAL : isFromCatalog(agent) ? CATALOG : undefined,
  availableFor: "agents created from source code",
};

/** Short kind shown next to an agent a page doesn't apply to. */
export function agentKindLabel(agent: AgentResponse): string {
  if (isExternalAgent(agent)) return "External";
  if (isFromCatalog(agent)) return "From catalog";
  return agent.agentType?.type === "agent-api" ? "API agent" : "From source";
}

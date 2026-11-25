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

/**
 * The left menu is the same at every level. Each entry says which levels its
 * page lives at; opening it anywhere else lands on that level's gate page
 * (`go/:navKey`), which asks for the missing project/agent or points upwards.
 */
export type NavLevel = "org" | "project" | "agent";

/** Broadest first. */
export const NAV_LEVELS: readonly NavLevel[] = ["org", "project", "agent"];

export const NAV_LEVEL_NAMES: Record<NavLevel, string> = {
  org: "organization",
  project: "project",
  agent: "agent",
};

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
  | "resources"
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
  pinBottom?: boolean;
  dividerBefore?: boolean;
}

export interface NavParams {
  orgId?: string;
  projectId?: string;
  agentId?: string;
  envId?: string;
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

export function isNavEntryActive(entry: NavEntry, pathname: string): boolean {
  const onPage = Object.values(entry.routes).some((route) =>
    (route?.activePaths ?? [`${route?.path}/*`]).some(
      (pattern) => !!matchPath(pattern, pathname),
    ),
  );
  if (onPage) return true;
  return NAV_LEVELS.some(
    (level) => matchPath(GATE_PATHS[level], pathname)?.params.navKey === entry.id,
  );
}

// Which agents each agent-level page applies to. Shared by the menu, the gate
// page and the route guard, so a direct URL gets the same answer as a click.
const isExternal = (agent: AgentResponse) =>
  agent.provisioning?.type === "external";
const isFromCatalog = (agent: AgentResponse) => !isExternal(agent) && !!agent.kindName;

const EXTERNAL = "External agents run outside the platform, so this page doesn't apply to them.";
const CATALOG = "Agents created from the catalog are built from their kind, so this page doesn't apply to them.";

export const AGENT_SUPPORT: Record<
  string,
  (agent: AgentResponse) => string | undefined
> = {
  build: (agent) =>
    isExternal(agent) ? EXTERNAL : isFromCatalog(agent) ? CATALOG : undefined,
  publish: (agent) =>
    isExternal(agent) ? EXTERNAL : isFromCatalog(agent) ? CATALOG : undefined,
  deploy: (agent) => (isExternal(agent) ? EXTERNAL : undefined),
  "try-it": (agent) => (isExternal(agent) ? EXTERNAL : undefined),
  "runtime-logs": (agent) => (isExternal(agent) ? EXTERNAL : undefined),
  "system-metrics": (agent) => (isExternal(agent) ? EXTERNAL : undefined),
  credentials: (agent) =>
    isExternal(agent) || agent.agentType?.type === "agent-api"
      ? undefined
      : "Credentials are only issued for API agents and external agents.",
};

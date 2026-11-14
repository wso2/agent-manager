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

// Plugin manifest. The shell imports this module eagerly, so it must not import
// the page components: they are reached only through the dynamic import() in
// each `load`, which keeps them in the package's own chunk.
import type { ComponentType } from "react";
import { RadioTower, Settings } from "@wso2/oxygen-ui-icons-react";
import {
  useEventsEnabled,
  useTokenScopes,
} from "@agent-management-platform/api-client";
import {
  useFeatureFlag,
  useFeatureManagement,
} from "@agent-management-platform/feature-management";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";

// Route-map shorthands (absolute paths, used for both routes and links).
const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

/**
 * Settings is shown only when profile management is enabled, and then only if
 * RBAC grants access to at least one of its identity surfaces.
 */
const useSettingsNav = () => {
  const profileManagementEnabled = useFeatureFlag("enableProfileManagement");
  const { hasAccess } = useFeatureManagement();
  return (
    profileManagementEnabled &&
    (hasAccess("member_read") || hasAccess("role_read") || hasAccess("group_read"))
  );
};

/** Events needs the service's event bus; /api/v1/config says whether it runs. */
const useEventsNav = () => useEventsEnabled();

/**
 * The org Events page also needs the events scopes; projects and agents show
 * theirs to anyone who can see them.
 */
const useOrgEventsNav = () => {
  const eventsEnabled = useEventsEnabled();
  const { enforced, scopes } = useTokenScopes();
  return (
    eventsEnabled &&
    (!enforced || scopes.has("amp:events:read") || scopes.has("amp:events:manage"))
  );
};

export const settingsPlugin: PluginManifest = {
  id: "settings",
  name: "Settings and events",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "settings-page",
      level: "org",
      path: orgNav.settings.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "org-events-page",
      level: "org",
      path: orgNav.events.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.OrgEvents as ComponentType,
        })),
    },
    {
      type: "route",
      id: "project-events-page",
      level: "project",
      path: orgNav.projects.children.events.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.ProjectEvents as ComponentType,
        })),
    },
    {
      type: "route",
      id: "agent-events-page",
      level: "agent",
      path: agentNav.events.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.AgentEvents as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "settings-nav",
      level: "org",
      section: "main",
      order: 50,
      label: "Settings",
      icon: Settings,
      route: orgNav.settings,
      pinBottom: true,
      useActivation: useSettingsNav,
    },
    {
      type: "nav-item",
      id: "org-events-nav",
      level: "org",
      section: "footer",
      order: 10,
      label: "Events",
      icon: RadioTower,
      route: orgNav.events,
      dividerBefore: true,
      useActivation: useOrgEventsNav,
    },
    {
      type: "nav-item",
      id: "project-events-nav",
      level: "project",
      section: "footer",
      order: 10,
      label: "Events",
      icon: RadioTower,
      route: orgNav.projects.children.events,
      dividerBefore: true,
      useActivation: useEventsNav,
    },
    {
      type: "nav-item",
      id: "agent-events-nav",
      level: "agent",
      section: "footer",
      order: 10,
      label: "Events",
      icon: RadioTower,
      route: agentNav.events,
      dividerBefore: true,
      useActivation: useEventsNav,
    },
  ],
};

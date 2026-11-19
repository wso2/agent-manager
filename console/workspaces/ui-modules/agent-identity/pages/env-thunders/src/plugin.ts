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
import { useFeatureAccess } from "@agent-management-platform/feature-management";
import { thunderInstancesMetadata } from "./metadata";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  PLUGIN_MOUNT_POINTS,
  type PluginManifest,
  type ResourceSummaryState,
} from "@agent-management-platform/types";
import { generatePath, useParams } from "react-router-dom";
import { Shield, Users } from "@wso2/oxygen-ui-icons-react";

// Route-map shorthands (absolute paths, used for both routes and links).
const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

const thunderInstances = orgNav.thunderInstances;

/** Agent identities are listed for users who can read them. */
const useAgentIdentitiesNav = () => useFeatureAccess("agent_identity_read");

const identityRoutes = absoluteRouteMap.children.org.children.thunderInstances.children;

/** The organization overview's Agent Identity Groups card (not counted). */
const useIdentityGroupsSummary = (): ResourceSummaryState => {
  const { orgId = "" } = useParams();
  return {
    countless: true,
    actionLabel: "Manage groups",
    to: generatePath(identityRoutes.groups.path, { orgId }),
  };
};

/** The organization overview's Agent Identity Roles card (not counted). */
const useIdentityRolesSummary = (): ResourceSummaryState => {
  const { orgId = "" } = useParams();
  return {
    countless: true,
    actionLabel: "Manage roles",
    to: generatePath(identityRoutes.roles.path, { orgId }),
  };
};

export const agentIdentityPlugin: PluginManifest = {
  id: "agent-identity",
  name: "Agent identity",
  apiVersion: PLUGIN_API_VERSION,
  // No feature-flag check here: the Agent Identity module's install detection
  // (enableAgentIdentity) already keeps these pages off until it is installed.
  contributions: [
    {
      // Organization overview: the Agent Identity Groups and Roles cards.
      type: "resource-summary",
      id: "identity-groups-summary",
      order: 50,
      title: "Agent Identity Groups",
      description: "Groups that collect agent identities so access is granted together.",
      icon: Users,
      useSummary: useIdentityGroupsSummary,
    },
    {
      type: "resource-summary",
      id: "identity-roles-summary",
      order: 60,
      title: "Agent Identity Roles",
      description: "Roles that decide what each agent identity is allowed to do.",
      icon: Shield,
      useSummary: useIdentityRolesSummary,
    },
    {
      // Agent overview, per environment: the agent's identity, roles and groups.
      type: "panel",
      id: "overview-roles-groups",
      mountPoint: PLUGIN_MOUNT_POINTS.agentOverviewEnvironment,
      order: 30,
      width: "half",
      load: () => import("./index").then((m) => ({ default: m.EnvAgentRolesGroupsSection as ComponentType })),
    },
    {
      type: "route",
      id: "org-page",
      level: "org",
      path: orgNav.thunderInstances.path + "/*",
      load: () =>
        import("./index").then((m) => ({
          default: m.thunderInstancesMetaData.levels!.organization as ComponentType,
        })),
    },
    {
      type: "route",
      id: "agent-id-page",
      level: "agent",
      path: agentNav.agentId.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.agentIdMetaData.component as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "groups-nav",
      level: "org",
      section: "agent-identities",
      order: 10,
      label: thunderInstancesMetadata.children.groups.title,
      icon: thunderInstancesMetadata.children.groups.icon,
      route: thunderInstances.children.groups,
      useActivation: useAgentIdentitiesNav,
    },
    {
      type: "nav-item",
      id: "roles-nav",
      level: "org",
      section: "agent-identities",
      order: 20,
      label: thunderInstancesMetadata.children.roles.title,
      icon: thunderInstancesMetadata.children.roles.icon,
      route: thunderInstances.children.roles,
      useActivation: useAgentIdentitiesNav,
    },
    {
      type: "nav-item",
      id: "agent-id-nav",
      level: "agent",
      section: "security",
      // Before Credentials (10).
      order: 0,
      label: "Agent ID",
      icon: thunderInstancesMetadata.icon,
      route: agentNav.agentId,
    },
  ],
};

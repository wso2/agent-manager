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
import { Rocket } from "@wso2/oxygen-ui-icons-react";
import { useRouteAgent } from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  PLUGIN_MOUNT_POINTS,
  type PluginActivationState,
  type PluginManifest,
} from "@agent-management-platform/types";

// Route-map shorthands (absolute paths, used for both routes and links).
const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

/** Internal agents run on the platform; external ones are only proxied. */
const useInternalAgent = (): PluginActivationState => {
  const { data: agent, isLoading } = useRouteAgent();
  if (isLoading) return undefined;
  return !!agent && agent.provisioning?.type !== "external";
};

/** An internal agent built from source, not created from a catalog kind. */
const useSourceBuiltAgent = (): PluginActivationState => {
  const { data: agent, isLoading } = useRouteAgent();
  if (isLoading) return undefined;
  return agent?.provisioning?.type === "internal" && !agent.kindName;
};

export const deployPlugin: PluginManifest = {
  id: "deploy",
  name: "Deploy",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      // Agent overview: the source repository and latest build.
      type: "panel",
      id: "overview-built-from",
      mountPoint: PLUGIN_MOUNT_POINTS.agentOverviewInfo,
      order: 10,
      useActivation: useSourceBuiltAgent,
      load: () => import("./index").then((m) => ({ default: m.AgentInfoPanel as ComponentType })),
    },
    {
      // Agent overview, per environment: the endpoint (invoke URL, auth, CORS).
      type: "panel",
      id: "overview-endpoint",
      mountPoint: PLUGIN_MOUNT_POINTS.agentOverviewEnvironment,
      order: 10,
      load: () => import("./index").then((m) => ({ default: m.EnvCapabilitiesSection as ComponentType })),
    },
    {
      // The deployed endpoint's HTTP resources; external agents have none.
      type: "panel",
      id: "overview-interface",
      mountPoint: PLUGIN_MOUNT_POINTS.agentOverviewEnvironment,
      order: 40,
      width: "half",
      useActivation: useInternalAgent,
      load: () => import("./index").then((m) => ({ default: m.EnvAgentInterfaceCard as ComponentType })),
    },
    {
      type: "route",
      id: "page",
      level: "agent",
      path: agentNav.deployment.path,
      load: () =>
        import("./index").then((m) => ({
          default: m.metaData.levels!.component as ComponentType,
        })),
    },
    {
      type: "nav-item",
      id: "nav",
      level: "agent",
      section: "lifecycle",
      order: 30,
      label: "Deploy",
      icon: Rocket,
      route: agentNav.deployment,
      useActivation: useInternalAgent,
    },
  ],
};

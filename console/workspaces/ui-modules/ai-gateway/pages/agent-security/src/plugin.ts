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
// `load`, which keeps them in their own chunk.
import { ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import {
  absoluteRouteMap,
  relativeRouteMap,
  PLUGIN_API_VERSION,
  type PluginActivationState,
  type PluginManifest,
} from "@agent-management-platform/types";
import { useRouteAgent } from "@agent-management-platform/api-client";

const agentRoutes = {
  relative: relativeRouteMap.children.org.children.projects.children.agents.children,
  absolute: absoluteRouteMap.children.org.children.projects.children.agents.children,
};

/**
 * Credentials (gateway API keys) apply to agents that are reached through the
 * gateway: every external agent (A2A proxy) and internal agents of type
 * agent-api. Reads the agent in the URL; the query is shared with the layout's
 * own useGetAgent call, so this adds no request.
 */
function useCredentialsActivation(): PluginActivationState {
  const { data: agent, isLoading } = useRouteAgent();
  if (isLoading) return undefined;
  if (!agent) return false;
  return (
    agent.provisioning?.type === "external" ||
    agent.agentType?.type === "agent-api"
  );
}

export const agentSecurityPlugin: PluginManifest = {
  id: "agent-security",
  name: "Agent credentials",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      // Reachable for any agent, as before; only the menu entry is gated.
      type: "route",
      id: "credentials-page",
      level: "agent",
      path:
        agentRoutes.relative.environment.path +
        "/" +
        agentRoutes.relative.environment.children.security.path,
      load: () =>
        import("./SecurityPage").then((m) => ({ default: m.SecurityPage })),
    },
    {
      type: "nav-item",
      id: "credentials-nav",
      level: "agent",
      section: "security",
      order: 10,
      label: "Credentials",
      icon: ShieldCheck,
      route: agentRoutes.absolute.environment.children.security,
      useActivation: useCredentialsActivation,
    },
  ],
};

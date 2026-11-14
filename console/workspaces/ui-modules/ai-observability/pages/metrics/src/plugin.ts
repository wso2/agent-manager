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
import { Gauge } from "@wso2/oxygen-ui-icons-react";
import { useRouteAgent } from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginActivationState,
  type PluginManifest,
} from "@agent-management-platform/types";

const observability =
  absoluteRouteMap.children.org.children.projects.children.agents.children
    .environment.children.observability;

/**
 * External agents (reached through the A2A proxy) have no runtime in the
 * platform, so they get traces only: no system metrics. Reads the agent in the
 * URL; the query is shared with the layout's own agent query.
 */
function useNotExternalAgent(): PluginActivationState {
  const { data: agent, isLoading } = useRouteAgent();
  if (isLoading) return undefined;
  if (!agent) return false;
  return agent.provisioning?.type !== "external";
}

export const metricsPlugin: PluginManifest = {
  id: "metrics",
  name: "System metrics",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "page",
      level: "agent",
      path: observability.children.metrics.path,
      load: () =>
        import("./Metrics.Component").then((m) => ({ default: m.MetricsComponent })),
    },
    {
      type: "nav-item",
      id: "nav",
      level: "agent",
      section: "observability",
      order: 30,
      label: "System metrics",
      icon: Gauge,
      route: observability.children.metrics,
      useActivation: useNotExternalAgent,
    },
  ],
};

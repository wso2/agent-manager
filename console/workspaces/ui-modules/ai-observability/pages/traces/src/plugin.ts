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
import type { ComponentType } from "react";
import { Workflow } from "@wso2/oxygen-ui-icons-react";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  PLUGIN_MOUNT_POINTS,
  type PluginManifest,
} from "@agent-management-platform/types";

const observability =
  absoluteRouteMap.children.org.children.projects.children.agents.children
    .environment.children.observability;

export const tracesPlugin: PluginManifest = {
  id: "traces",
  name: "Traces",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      // Agent overview, per environment: recent traces and latency.
      type: "panel",
      id: "overview-observability",
      mountPoint: PLUGIN_MOUNT_POINTS.agentOverviewEnvironment,
      order: 70,
      load: () => import("./index").then((m) => ({ default: m.EnvObservabilitySection as ComponentType })),
    },
    {
      type: "route",
      id: "page",
      level: "agent",
      path: observability.children.traces.path,
      load: () =>
        import("./Traces.Component").then((m) => ({ default: m.TracesComponent })),
    },
    {
      type: "nav-item",
      id: "nav",
      level: "agent",
      section: "observability",
      order: 10,
      label: "Traces",
      icon: Workflow,
      route: observability.children.traces,
    },
  ],
};

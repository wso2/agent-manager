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
// `load`, which keeps them in the package's own chunk.
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  PLUGIN_MOUNT_POINTS,
  type PluginManifest,
} from "@agent-management-platform/types";
import { InternalAgentIcon } from "@agent-management-platform/views";

const newAgent = absoluteRouteMap.children.org.children.projects.children.newAgent;
const fromSource = newAgent.children.create.children.source;

// A platform-hosted agent is built from source and deployed by the agent
// runtime, so this module offers the agent type, its create form, and the
// samples that seed that form.
export const platformAgentPlugin: PluginManifest = {
  id: "platform-agent",
  name: "Platform-hosted agents",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "page",
      level: "project",
      path: fromSource.path,
      load: () => import("./index").then((m) => ({ default: m.PlatformAgentPage })),
    },
    {
      type: "agent-type",
      id: "build-new",
      // The wizard used to ask "build new" and then "from source".
      wizardSteps: ["build-new", "from-source"],
      order: 20,
      title: "Platform-Hosted Agent",
      description:
        "Deploy and manage agents with full lifecycle support, including built-in CI/CD, scaling, observability, and governance.",
      ctaLabel: "Deploy a new agent",
      illustration: InternalAgentIcon,
      route: fromSource,
    },
    {
      type: "panel",
      id: "samples",
      mountPoint: PLUGIN_MOUNT_POINTS.addNewAgentOptions,
      load: () => import("./index").then((m) => ({ default: m.NewAgentSampleCard })),
    },
  ],
};

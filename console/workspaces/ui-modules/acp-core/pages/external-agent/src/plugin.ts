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
  type PluginManifest,
} from "@agent-management-platform/types";
import { ExternalAgentIcon } from "@agent-management-platform/views";

const newAgent = absoluteRouteMap.children.org.children.projects.children.newAgent;

// An externally-hosted agent runs outside the platform and only registers
// with it, which the core does; this page offers the agent type and its
// register form.
export const externalAgentPlugin: PluginManifest = {
  id: "external-agent",
  name: "Externally-hosted agents",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "page",
      level: "project",
      path: newAgent.children.connect.path,
      load: () => import("./index").then((m) => ({ default: m.ExternalAgentPage })),
    },
    {
      // The id is also the create wizard's telemetry step (see wizardSteps).
      type: "agent-type",
      id: "connect-existing",
      order: 10,
      title: "Externally-Hosted Agent",
      description:
        "Connect an existing agent running outside the platform and enable observability and governance.",
      ctaLabel: "Register an agent",
      illustration: ExternalAgentIcon,
      route: newAgent.children.connect,
    },
  ],
};

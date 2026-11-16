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
// each `load`, which keeps them in their own chunks.
//
// One plugin, two features: Environments and Deployment pipelines are gated
// separately, so each contribution carries its own activation hook.
import { Layers, Workflow } from "@wso2/oxygen-ui-icons-react";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";
import {
  useEnvironmentFeatures,
  usePipelineFeatures,
} from "@agent-management-platform/feature-management";

// Activation hooks: hidden when the resource is managed outside the console or
// the user lacks read access to it.
const useEnvironmentsActivation = () => useEnvironmentFeatures().canView;
const usePipelinesActivation = () => usePipelineFeatures().canView;

export const deploymentPipelinesPlugin: PluginManifest = {
  id: "deployment-pipelines",
  name: "Environments and deployment pipelines",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    {
      type: "route",
      id: "environments-page",
      level: "org",
      path: absoluteRouteMap.children.org.children.environments.path + "/*",
      useActivation: useEnvironmentsActivation,
      load: () =>
        import("./Environments.Organization").then((m) => ({
          default: m.EnvironmentsOrganization,
        })),
    },
    {
      type: "nav-item",
      id: "environments-nav",
      level: "org",
      section: "infrastructure",
      order: 20,
      label: "Environments",
      icon: Layers,
      route: absoluteRouteMap.children.org.children.environments,
      useActivation: useEnvironmentsActivation,
    },
    {
      type: "route",
      id: "pipelines-page",
      level: "org",
      path:
        absoluteRouteMap.children.org.children.deploymentPipelines.path + "/*",
      useActivation: usePipelinesActivation,
      load: () =>
        import("./DeploymentPipelines.Organization").then((m) => ({
          default: m.DeploymentPipelinesOrganization,
        })),
    },
    {
      type: "nav-item",
      id: "pipelines-nav",
      level: "org",
      section: "infrastructure",
      order: 30,
      label: "Deployment pipelines",
      icon: Workflow,
      route: absoluteRouteMap.children.org.children.deploymentPipelines,
      useActivation: usePipelinesActivation,
    },
  ],
};

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
import { Code, MonitorCheck } from "@wso2/oxygen-ui-icons-react";
import { useFeatureAccess } from "@agent-management-platform/feature-management";
import {
  absoluteRouteMap,
  PLUGIN_API_VERSION,
  type PluginManifest,
} from "@agent-management-platform/types";

// Route-map shorthands (absolute paths, used for both routes and links).
const orgNav = absoluteRouteMap.children.org.children;
const agentNav = orgNav.projects.children.agents.children;

const evaluators = orgNav.evaluators;
// Monitor pages live under environment/:envId/evaluation/monitor.
const monitor = agentNav.environment.children.evaluation.children.monitor;

const evaluatorRoute = (id: string, path: string, page: string) => ({
  type: "route" as const,
  id,
  level: "org" as const,
  path,
  load: () =>
    import("./index").then((m) => ({
      default: m.metaData.pages.organization[page]!.component as ComponentType,
    })),
});

const monitorRoute = (id: string, path: string, page: string) => ({
  ...evaluatorRoute(id, path, page),
  level: "agent" as const,
});

export const evalPlugin: PluginManifest = {
  id: "eval",
  name: "Evaluation",
  apiVersion: PLUGIN_API_VERSION,
  contributions: [
    evaluatorRoute("evaluators-page", evaluators.path, "evalEvaluators"),
    evaluatorRoute(
      "create-evaluator-page",
      evaluators.children.create.path,
      "createEvaluator",
    ),
    evaluatorRoute(
      "edit-evaluator-page",
      evaluators.children.edit.path,
      "editEvaluator",
    ),
    evaluatorRoute(
      "view-evaluator-page",
      evaluators.children.view.path,
      "viewEvaluator",
    ),
    monitorRoute("monitors-page", monitor.path, "evalMonitors"),
    monitorRoute(
      "create-monitor-page",
      monitor.children.create.path,
      "createMonitor",
    ),
    monitorRoute(
      "edit-monitor-page",
      monitor.children.edit.path,
      "editMonitor",
    ),
    monitorRoute(
      "compare-monitor-page",
      monitor.children.compare.path,
      "compareMonitor",
    ),
    monitorRoute(
      "view-monitor-page",
      monitor.children.view.path + "/*",
      "viewMonitor",
    ),
    {
      type: "nav-item",
      id: "evaluators-nav",
      level: "org",
      section: "evaluation",
      order: 10,
      label: "Evaluators",
      icon: Code,
      route: orgNav.evaluators,
      useActivation: () => useFeatureAccess("evaluator_read"),
    },
    {
      type: "nav-item",
      id: "monitors-nav",
      level: "agent",
      section: "evaluation",
      order: 10,
      label: "Monitors",
      icon: MonitorCheck,
      route: agentNav.environment.children.evaluation.children.monitor,
    },
  ],
};

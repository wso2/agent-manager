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

// AI Observability UI module. Lists the module's page plugins; the console registers
// modules, and the server turns a whole module or a single page on or
// off (GET /api/v1/ui-manifest).
//
// Import only each page package's `/plugin` entry: the package root pulls in
// its page components and would put them in the shell's main chunk.
import {
  PLUGIN_API_VERSION,
  type UIModuleInstallState,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import { isObserverConfigured } from "@agent-management-platform/api-client";
import { tracesPlugin } from "@agent-management-platform/traces/plugin";
import { logsPlugin } from "@agent-management-platform/logs/plugin";
import { metricsPlugin } from "@agent-management-platform/metrics/plugin";
import { evalPlugin } from "@agent-management-platform/eval/plugin";

/**
 * Installed once something sets the observer URL (setObserverBaseUrl). The
 * console no longer registers this module and nothing sets the URL: the
 * source stays for the observability module that will replace it.
 */
function useDetectObserver(): UIModuleInstallState | undefined {
  return isObserverConfigured() ? "installed" : "not-installed";
}

export const aiObservabilityModule: UIModuleManifest = {
  id: "ai-observability",
  name: "AI Observability",
  description: "Traces, runtime logs, system metrics and evaluation.",
  apiVersion: PLUGIN_API_VERSION,
  installation: {
    chart: "wso2-amp-observability-extension",
    useDetect: useDetectObserver,
    steps: [
      {
        title: "Install the observability extension",
        description:
          "Deploys the Agent Manager Observer, which collects traces, logs and metrics.",
        command:
          "helm upgrade --install wso2-amp-observability-extension " +
          "oci://{chartRegistry}/wso2-amp-observability-extension --version {chartVersion} " +
          "--namespace openchoreo-observability-plane --create-namespace",
      },
      {
        title: "Install the evaluation extension (optional)",
        description: "Needed for evaluation monitors; evaluators work without it.",
        command:
          "helm upgrade --install amp-evaluation-workflows-extension " +
          "oci://{chartRegistry}/wso2-amp-evaluation-extension --version {chartVersion} " +
          "--namespace openchoreo-workflow-plane",
      },
    ],
  },
  plugins: [tracesPlugin, logsPlugin, metricsPlugin, evalPlugin],
};

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

// Agent Identity UI module. Lists the module's page plugins; the console registers
// modules, and operators turn a whole module or a single page off with the
// runtime config's uiModules.disabled list (or in the Module manager).
//
// Import only each page package's `/plugin` entry: the package root pulls in
// its page components and would put them in the shell's main chunk.
import {
  PLUGIN_API_VERSION,
  type UIModuleInstallState,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import { useFeatureFlag } from "@agent-management-platform/feature-management";
import { agentIdentityPlugin } from "@agent-management-platform/env-thunders/plugin";

/** Installed when the deployment turns agent identity on (featureFlags.enableAgentIdentity). */
function useDetectAgentIdentity(): UIModuleInstallState {
  return useFeatureFlag("enableAgentIdentity") ? "installed" : "not-installed";
}

export const agentIdentityModule: UIModuleManifest = {
  id: "agent-identity",
  name: "Agent Identity",
  description: "Agent IDs and the groups and roles agents can hold.",
  apiVersion: PLUGIN_API_VERSION,
  installation: {
    chart: "wso2-amp-thunder-extension",
    useDetect: useDetectAgentIdentity,
    steps: [
      {
        title: "Install the Thunder identity provider extension",
        description: "Deploys the identity provider that issues agent IDs.",
        command:
          "helm upgrade --install amp-thunder-extension " +
          "oci://{chartRegistry}/wso2-amp-thunder-extension --version {chartVersion} " +
          "--namespace amp-thunder --create-namespace",
      },
      {
        title: "Turn agent identity on in the console",
        description:
          "Set console.config.featureFlags.enableAgentIdentity to true in the agent manager " +
          "Helm values and upgrade the release.",
      },
    ],
  },
  plugins: [agentIdentityPlugin],
};

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

// ACP Core UI module. Lists the module's page plugins; the console registers
// modules, and operators turn a whole module or a single page off with the
// runtime config's uiModules.disabled list (or in the Module manager).
//
// Import only each page package's `/plugin` entry: the package root pulls in
// its page components and would put them in the shell's main chunk.
import {
  PLUGIN_API_VERSION,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import { overviewPlugin } from "@agent-management-platform/overview/plugin";
import { resourceListingPlugin } from "@agent-management-platform/level-resource-listing/plugin";
import { addNewProjectPlugin } from "@agent-management-platform/add-new-project/plugin";
import { addNewAgentPlugin } from "@agent-management-platform/add-new-agent/plugin";
import { settingsPlugin } from "@agent-management-platform/settings/plugin";

export const acpCoreModule: UIModuleManifest = {
  id: "acp-core",
  name: "ACP Core",
  description: "Overview, project and agent listings, create flows, settings and events.",
  apiVersion: PLUGIN_API_VERSION,
  // Ships with every console: always installed, cannot be turned off as a whole.
  core: true,
  plugins: [
    overviewPlugin,
    resourceListingPlugin,
    addNewProjectPlugin,
    addNewAgentPlugin,
    settingsPlugin,
  ],
};

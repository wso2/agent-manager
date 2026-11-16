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

// Agent Runtime UI module. Lists the module's page plugins; the console registers
// modules, and operators turn a whole module or a single page off in
// public/ui-modules.js (apps/web-ui).
//
// Import only each page package's `/plugin` entry: the package root pulls in
// its page components and would put them in the shell's main chunk.
import {
  PLUGIN_API_VERSION,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import { configureAgentPlugin } from "@agent-management-platform/configure-agent/plugin";
import { buildPlugin } from "@agent-management-platform/build/plugin";
import { deployPlugin } from "@agent-management-platform/deploy/plugin";
import { testPlugin } from "@agent-management-platform/test/plugin";
import { deploymentPipelinesPlugin } from "@agent-management-platform/deployment-pipelines/plugin";

export const agentRuntimeModule: UIModuleManifest = {
  id: "agent-runtime",
  name: "Agent Runtime",
  description: "Configure, build, deploy and try agents; environments and deployment pipelines.",
  apiVersion: PLUGIN_API_VERSION,
  plugins: [configureAgentPlugin, buildPlugin, deployPlugin, testPlugin, deploymentPipelinesPlugin],
};

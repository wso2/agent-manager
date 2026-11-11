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

import type { PluginManifest } from "@agent-management-platform/types";
// Import only the `/plugin` entry of each package. The package root pulls in
// its page components and would put them in the shell's main chunk.
import { gatewaysPlugin } from "@agent-management-platform/gateways/plugin";
import { deploymentPipelinesPlugin } from "@agent-management-platform/deployment-pipelines/plugin";

/**
 * Plugins that ship with the console. Each can be turned off with the runtime
 * config's `plugins.disabled` list; extra plugins can be passed to CoreUI.
 */
export const builtinPlugins: PluginManifest[] = [
  gatewaysPlugin,
  deploymentPipelinesPlugin,
];

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
import { overviewPlugin } from "@agent-management-platform/overview/plugin";
import { resourceListingPlugin } from "@agent-management-platform/level-resource-listing/plugin";
import { addNewProjectPlugin } from "@agent-management-platform/add-new-project/plugin";
import { addNewAgentPlugin } from "@agent-management-platform/add-new-agent/plugin";
import { configureAgentPlugin } from "@agent-management-platform/configure-agent/plugin";
import { buildPlugin } from "@agent-management-platform/build/plugin";
import { deployPlugin } from "@agent-management-platform/deploy/plugin";
import { testPlugin } from "@agent-management-platform/test/plugin";
import { agentKindPlugin } from "@agent-management-platform/agent-kind/plugin";
import { settingsPlugin } from "@agent-management-platform/settings/plugin";
import { llmProvidersPlugin } from "@agent-management-platform/llm-providers/plugin";
import { mcpProxiesPlugin } from "@agent-management-platform/mcp-proxies/plugin";
import { gatewaysPlugin } from "@agent-management-platform/gateways/plugin";
import { deploymentPipelinesPlugin } from "@agent-management-platform/deployment-pipelines/plugin";
import { agentSecurityPlugin } from "@agent-management-platform/agent-security/plugin";
import { agentIdentityPlugin } from "@agent-management-platform/env-thunders/plugin";
import { tracesPlugin } from "@agent-management-platform/traces/plugin";
import { logsPlugin } from "@agent-management-platform/logs/plugin";
import { metricsPlugin } from "@agent-management-platform/metrics/plugin";
import { evalPlugin } from "@agent-management-platform/eval/plugin";

/**
 * Plugins that ship with the console. Each can be turned off with the runtime
 * config's `plugins.disabled` list; extra plugins can be passed to CoreUI.
 */
export const builtinPlugins: PluginManifest[] = [
  // Core navigation and lifecycle
  overviewPlugin,
  resourceListingPlugin,
  addNewProjectPlugin,
  addNewAgentPlugin,
  configureAgentPlugin,
  buildPlugin,
  deployPlugin,
  testPlugin,
  settingsPlugin,
  // Resources
  llmProvidersPlugin,
  mcpProxiesPlugin,
  // Infrastructure
  gatewaysPlugin,
  deploymentPipelinesPlugin,
  // Security and identity ("agent-identity" module)
  agentSecurityPlugin,
  agentIdentityPlugin,
  // Catalog ("catalog" module)
  agentKindPlugin,
  // Observability module
  tracesPlugin,
  logsPlugin,
  metricsPlugin,
  // Evaluation module
  evalPlugin,
];

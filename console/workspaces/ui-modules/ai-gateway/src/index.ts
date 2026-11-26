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

// AI Gateway UI module. Lists the module's page plugins; the console registers
// modules, and the server turns a whole module or a single page on or
// off (GET /api/v1/ui-manifest).
//
// Import only each page package's `/plugin` entry: the package root pulls in
// its page components and would put them in the shell's main chunk.
import {
  PLUGIN_API_VERSION,
  type UIModuleManifest,
} from "@agent-management-platform/types";
import { gatewaysPlugin } from "@agent-management-platform/gateways/plugin";
import { llmProvidersPlugin } from "@agent-management-platform/llm-providers/plugin";
import { mcpProxiesPlugin } from "@agent-management-platform/mcp-proxies/plugin";
import { agentSecurityPlugin } from "@agent-management-platform/agent-security/plugin";
import { configureAgentPlugin } from "@agent-management-platform/configure-agent/plugin";
import { aiCostManagementPlugin } from "@agent-management-platform/ai-cost-management/plugin";

export const aiGatewayModule: UIModuleManifest = {
  id: "ai-gateway",
  name: "AI Gateway",
  description:
    "Gateways, LLM service providers, MCP servers, agent credentials, each agent's " +
    "LLM, MCP and A2A configuration, and cost and budgets.",
  apiVersion: PLUGIN_API_VERSION,
  installation: {
    chart: "wso2-amp-api-platform-gateway-extension",
    steps: [
      {
        title: "Install the AI gateway into a data plane",
        description:
          "Deploys the API Platform gateway and registers it with the agent manager. " +
          "You can also add a gateway per environment from Infrastructure > Gateways.",
        command:
          "helm upgrade --install amp-ai-gateway " +
          "oci://{chartRegistry}/wso2-amp-api-platform-gateway-extension --version {chartVersion} " +
          "--namespace openchoreo-data-plane",
      },
    ],
  },
  plugins: [
    gatewaysPlugin,
    llmProvidersPlugin,
    mcpProxiesPlugin,
    agentSecurityPlugin,
    configureAgentPlugin,
    aiCostManagementPlugin,
  ],
};

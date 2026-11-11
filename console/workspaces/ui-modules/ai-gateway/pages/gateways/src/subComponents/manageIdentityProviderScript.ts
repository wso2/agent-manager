/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  getAgentManagerUrl,
  getAmpChartRegistryHost,
  getInstalledChartVersion,
  getPrivateRegistryEnv,
  getRawScriptUrl,
  shellQuote,
} from "@agent-management-platform/shared-component";

const SCRIPT_NAME = "manage-identity-provider.sh";

export type ManageIdentityProviderMode = "upsert" | "delete";

export interface ScriptInputs {
  orgId: string;
  envName: string;
  gatewayId: string;
  name: string;
  issuer: string;
  jwksUri: string;
  skipTlsVerify: boolean;
  mode: ManageIdentityProviderMode;
  token: string;
}

export function buildScript(i: ScriptInputs): string {
  // The dev-container quickstart and a real cluster both install the gateway
  // extension from the same OCI chart, so one command works for both: curl the
  // release-pinned script and upgrade the OCI chart at the deployed version.
  const actionEnvLines =
    i.mode === "delete"
      ? ["    ACTION=delete \\"]
      : [
          `    IDP_ISSUER=${shellQuote(i.issuer || "<issuer>")} \\`,
          `    IDP_JWKS_URI=${shellQuote(i.jwksUri || "<jwks-uri>")} \\`,
          ...(i.skipTlsVerify ? ["    IDP_SKIP_TLS_VERIFY=true \\"] : []),
        ];
  // A private install upgrades the gateway extension chart from its own
  // registry, which helm has to be signed in to first. The script needs no pull
  // secrets, so it can still be piped.
  const { CHART_REF: chartRef } = getPrivateRegistryEnv();
  const chartHost = getAmpChartRegistryHost();
  const lines = [
    ...(chartRef ? [`helm registry login ${shellQuote(chartHost)} \\`] : []),
    `${chartRef ? "  && " : ""}curl -fsSL ${shellQuote(getRawScriptUrl(SCRIPT_NAME))} \\`,
    `  | ORG_NAME=${shellQuote(i.orgId || "<org>")} \\`,
    `    ENV_NAME=${shellQuote(i.envName || "<env-name>")} \\`,
    `    GATEWAY_ID=${shellQuote(i.gatewayId || "<gateway-id>")} \\`,
    `    AGENT_MANAGER_TOKEN=${shellQuote(i.token)} \\`,
    `    AGENT_MANAGER_URL=${shellQuote(getAgentManagerUrl())} \\`,
    `    CHART_VERSION=${shellQuote(getInstalledChartVersion())} \\`,
    ...(chartRef ? [`    CHART_REF=${shellQuote(chartRef)} \\`] : []),
    `    IDP_NAME=${shellQuote(i.name || "<identity-provider-name>")} \\`,
    ...actionEnvLines,
    "    bash",
  ];
  return lines.join("\n");
}

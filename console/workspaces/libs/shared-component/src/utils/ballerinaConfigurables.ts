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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { AGENTID_ENV_VAR_ROWS, type EnvVarReferenceRow } from "./mcpEnvVarSpec";

export const BALLERINA_CONFIG_VAR_PREFIX = "BAL_CONFIG_VAR_";

/**
 * The env var Ballerina binds a configurable from: `BAL_CONFIG_VAR_` plus the
 * configurable's name upper-cased, with no separators added
 * (`isAdmin` → `BAL_CONFIG_VAR_ISADMIN`). The mapping is one-way — upper-casing
 * loses the camelCase — so the configurable name is the source of truth.
 * Ballerina refuses to start on a BAL_CONFIG_VAR_* var with no configurable.
 */
export function ballerinaConfigVarName(configurableName: string): string {
  return `${BALLERINA_CONFIG_VAR_PREFIX}${configurableName.toUpperCase()}`;
}

const AGENTID_CONFIGURABLE_NAMES: Record<string, string> = {
  clientId: "ampAgentidClientId",
  clientSecret: "ampAgentidClientSecret",
  tokenEndpoint: "ampAgentidTokenEndpoint",
  scopes: "ampAgentidScopes",
};

/**
 * The Ballerina configurables a program declares to receive its AgentID
 * credentials when agentIdAsBallerinaConfigurables is enabled. Mirrors the
 * BalConfigVarAgentID* names in agent-manager-service.
 */
export const AGENTID_BALLERINA_CONFIGURABLE_ROWS: readonly EnvVarReferenceRow[] =
  AGENTID_ENV_VAR_ROWS.map((row) => ({ ...row, name: AGENTID_CONFIGURABLE_NAMES[row.key] }));

/**
 * The AgentID env vars injected in an environment: the BAL_CONFIG_VAR_* names of
 * the Ballerina configurables when agentIdAsBallerinaConfigurables is enabled
 * there, the plain AMP_AGENTID_* names otherwise. Only one set is injected.
 */
export function agentIdEnvVarRows(
  asBallerinaConfigurables: boolean | undefined,
): EnvVarReferenceRow[] {
  if (!asBallerinaConfigurables) return [...AGENTID_ENV_VAR_ROWS];
  return AGENTID_BALLERINA_CONFIGURABLE_ROWS.map((row) => ({
    ...row,
    name: ballerinaConfigVarName(row.name),
  }));
}

/**
 * The configurable an injected env var binds, preferring `preferred` (e.g. the
 * camelCase name it was generated from) when it maps to that var. Otherwise the
 * suffix after the prefix, lower-cased with its underscores kept
 * (`BAL_CONFIG_VAR_INJECT1_APIKEY` → `inject1_apikey`): Ballerina only
 * upper-cases the configurable's name, so an underscore in the var must be in
 * the configurable too — a camelCase `inject1Apikey` would leave the var
 * unused and stop the program. Undefined for a non-BAL_CONFIG_VAR_ name.
 */
export function ballerinaConfigurableFor(
  envVarName: string,
  preferred?: string,
): string | undefined {
  if (preferred && ballerinaConfigVarName(preferred) === envVarName) return preferred;
  if (!envVarName.startsWith(BALLERINA_CONFIG_VAR_PREFIX)) return undefined;
  return envVarName.slice(BALLERINA_CONFIG_VAR_PREFIX.length).toLowerCase() || undefined;
}

/**
 * `configurable string <name> = "";` for each configurable. Each defaults to
 * "" so the program still starts if a value arrives after the pod (e.g. AgentID
 * credentials provisioned after the first build).
 */
export function ballerinaConfigurableSnippet(configurableNames: readonly string[]): string {
  return configurableNames.map((name) => `configurable string ${name} = "";`).join("\n");
}

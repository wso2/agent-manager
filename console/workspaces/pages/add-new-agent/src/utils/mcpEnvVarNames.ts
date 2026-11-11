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

import type { MCPProxyFormEntry } from "../form/schema";

/** The default URL variable name for the nth MCP entry on the create form. */
export function defaultMCPUrlVarName(agentNameUpper: string, index: number): string {
  return `${agentNameUpper}_MCP_${index + 1}_URL`;
}

/** The default API key variable name for the nth MCP entry on the create form. */
export function defaultMCPApiKeyVarName(agentNameUpper: string, index: number): string {
  return `${agentNameUpper}_MCP_${index + 1}_API_KEY`;
}

/**
 * Whether this entry's endpoint has an API key variable the user names. Only an
 * "apiKey" endpoint does — OAuth mints credentials server-side and an unsecured
 * endpoint needs none (issue #1597).
 *
 * Undefined authenticationType means the proxy's security has not resolved yet, so
 * there is no API key variable to account for either.
 */
export function mcpEntryHasAPIKeyVar(entry: MCPProxyFormEntry): boolean {
  return entry.authenticationType === "apiKey";
}

/**
 * The environment variable names one MCP entry occupies, used to detect collisions
 * against other configs on the form. An entry with no API key variable must not
 * reserve that name, or it would block it for an unrelated config while never
 * using it.
 */
export function mcpEntryVarNames(
  entry: MCPProxyFormEntry,
  index: number,
  agentNameUpper: string,
): string[] {
  const names = [entry.urlVarName ?? defaultMCPUrlVarName(agentNameUpper, index)];
  if (mcpEntryHasAPIKeyVar(entry)) {
    names.push(entry.apikeyVarName ?? defaultMCPApiKeyVarName(agentNameUpper, index));
  }
  return names;
}

/**
 * Whether any MCP entry has a proxy selected but its security still unresolved —
 * the fetch is in flight or failed. Creating in that state would submit whichever
 * env vars the unknown default happens to imply, so the flows block on it
 * (issue #1597).
 */
export function hasUnresolvedMCPSecurity(entries: MCPProxyFormEntry[]): boolean {
  return entries.some(
    (entry) =>
      entry.authenticationType === undefined &&
      Object.values(entry.selectedProxyByEnv).some((proxy) => !!proxy),
  );
}

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

import type { EnvVarKey } from "@agent-management-platform/shared-component";

// The key list itself now lives in shared-component alongside resolveMCPEnvVarSpec,
// which decides which of these keys a given endpoint's security actually needs.
// Re-exported here so the LLM-provider and MCP pages keep their existing imports.
export {
  ENV_VAR_KEYS,
  type EnvVarKey,
} from "@agent-management-platform/shared-component";

export function generateEnvVarNames(prefix: string): Record<EnvVarKey, string> {
  let sanitized = prefix.replace(/[^A-Za-z0-9_]/g, "_").toUpperCase();
  if (sanitized.length > 0 && sanitized[0] >= "0" && sanitized[0] <= "9") {
    sanitized = "_" + sanitized;
  }
  return {
    url: sanitized ? `${sanitized}_URL` : "URL",
    apikey: sanitized ? `${sanitized}_API_KEY` : "API_KEY",
  };
}

export function generateUniqueConfigName(
  identifier: string,
  fallback: string,
  existingNames: string[],
): string {
  const base = (identifier || fallback)
    .replace(/[^A-Za-z0-9-]/g, "-")
    .toLowerCase();
  if (!existingNames.includes(base)) return base;
  let i = 2;
  while (existingNames.includes(`${base}-${i}`)) i++;
  return `${base}-${i}`;
}

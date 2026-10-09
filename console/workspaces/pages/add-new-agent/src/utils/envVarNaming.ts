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

import { ballerinaConfigVarName } from "@agent-management-platform/shared-component";

export type EnvVarNameKind = "llmUrl" | "llmApiKey" | "mcpUrl" | "mcpApiKey";

/**
 * Default env var names for the nth (0-based) LLM provider / MCP server entry
 * on the create form, and — for a Ballerina agent — the camelCase configurable
 * each default name binds.
 */
export interface EnvVarNaming {
  name(kind: EnvVarNameKind, index: number): string;
  /** The configurable `name(kind, index)` binds; undefined for non-Ballerina agents. */
  configurable(kind: EnvVarNameKind, index: number): string | undefined;
}

/** "My agent-1" → "myAgent1"; empty when the name has no letters or digits. */
export function toCamelCase(displayName: string | undefined): string {
  const words = (displayName ?? "").split(/[^A-Za-z0-9]+/).filter(Boolean);
  return words
    .map((word, i) => {
      if (i === 0) {
        return word === word.toUpperCase()
          ? word.toLowerCase()
          : word[0].toLowerCase() + word.slice(1);
      }
      return word[0].toUpperCase() + word.slice(1);
    })
    .join("");
}

const CONFIGURABLE_SUFFIX: Record<EnvVarNameKind, (n: number) => string> = {
  llmUrl: (n) => `${n}Url`,
  llmApiKey: (n) => `${n}ApiKey`,
  mcpUrl: (n) => `Mcp${n}Url`,
  mcpApiKey: (n) => `Mcp${n}ApiKey`,
};

const ENV_VAR_SUFFIX: Record<EnvVarNameKind, (n: number) => string> = {
  llmUrl: (n) => `_${n}_URL`,
  llmApiKey: (n) => `_${n}_API_KEY`,
  mcpUrl: (n) => `_MCP_${n}_URL`,
  mcpApiKey: (n) => `_MCP_${n}_API_KEY`,
};

/**
 * Ballerina agents read these values as configurables, bound from
 * BAL_CONFIG_VAR_<NAME> with <NAME> the configurable's name upper-cased. So a
 * Ballerina agent's defaults start from an idiomatic camelCase configurable
 * (`myAgent1Url`) and derive the env var from it (`BAL_CONFIG_VAR_MYAGENT1URL`);
 * every other agent keeps `MY_AGENT_1_URL`.
 */
export function envVarNaming(
  displayName: string | undefined,
  language: string | undefined,
): EnvVarNaming {
  if (language === "ballerina") {
    let base = toCamelCase(displayName) || "agent";
    if (/^[0-9]/.test(base)) base = `agent${base}`;
    const configurable = (kind: EnvVarNameKind, index: number) =>
      `${base}${CONFIGURABLE_SUFFIX[kind](index + 1)}`;
    return {
      name: (kind, index) => ballerinaConfigVarName(configurable(kind, index)),
      configurable,
    };
  }
  const upper = displayName ? displayName.toUpperCase().replace(/[^A-Z0-9]/g, "_") : "AGENT";
  return {
    name: (kind, index) => `${upper}${ENV_VAR_SUFFIX[kind](index + 1)}`,
    configurable: () => undefined,
  };
}

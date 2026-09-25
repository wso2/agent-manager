/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import {
  AgentKindConfigSchemaItem,
  CreateAgentRequest,
  MCPConfigRequest,
  ModelConfigRequest,
  OrgProjPathParams,
  PromotionPath,
} from "@agent-management-platform/types";
import {
  AddAgentFormValues,
  CreateAgentFormValues,
  LLMProviderFormEntry,
  MCPProxyFormEntry,
} from "../form/schema";
import { mcpEntryHasAPIKeyVar } from "./mcpEnvVarNames";

export interface CatalogEnvSeed {
  env: { key: string; value: string; isSensitive: boolean }[];
  lockedEnvKeys: Set<string>;
  /**
   * Keys that are both kind-declared secrets AND have a real default set — only
   * these should render as a locked/masked "existing secret" field. A mandatory
   * secret with no default must stay a normal, empty, editable field, since there
   * is nothing to fall back to if the user leaves it alone — the kind's config
   * schema response can't tell those two cases apart by isSecret alone, since a
   * secret's real default is never sent to the client (only whether one exists).
   */
  kindSecretKeysWithDefault: Set<string>;
}

export function deriveCatalogEnvSeed(schema: AgentKindConfigSchemaItem[]): CatalogEnvSeed {
  return {
    env: schema.map((item) => ({
      key: item.name,
      value: item.isSecret ? "" : (item.defaultValue ?? ""),
      isSensitive: item.isSecret,
    })),
    lockedEnvKeys: new Set(schema.map((item) => item.name)),
    kindSecretKeysWithDefault: new Set(
      schema.filter((item) => item.isSecret && !!item.defaultValue).map((item) => item.name),
    ),
  };
}

export function findLowestEnvironmentName(
  promotionPaths: PromotionPath[] = [],
): string | undefined {
  const targetEnvironments = new Set<string>();
  for (const path of promotionPaths) {
    for (const target of path.targetEnvironmentRefs) {
      targetEnvironments.add(target.name);
    }
  }

  return promotionPaths.find(
    (path) => !targetEnvironments.has(path.sourceEnvironmentRef),
  )?.sourceEnvironmentRef;
}

export function hasMultipleEnvironments(
  promotionPaths: PromotionPath[] = [],
): boolean {
  const names = new Set<string>();
  for (const path of promotionPaths) {
    if (path.sourceEnvironmentRef) names.add(path.sourceEnvironmentRef);
    for (const target of path.targetEnvironmentRefs ?? []) {
      if (target.name) names.add(target.name);
    }
    if (names.size > 1) return true;
  }
  return false;
}

function buildOneModelConfig(
  entry: LLMProviderFormEntry,
  initialEnvironmentName: string | undefined,
): ModelConfigRequest | null {
  // The create-side config applies only to the component's initial environment.
  const provider = initialEnvironmentName
    ? entry.selectedProviderByEnv[initialEnvironmentName]
    : null;
  if (!provider) return null;

  const policies =
    entry.guardrails.length > 0
      ? entry.guardrails.map((g) => ({
          name: g.name,
          version: g.version,
          paths: [{ path: "/*", methods: ["*"], params: g.settings ?? {} }],
        }))
      : undefined;

  const environmentVariables = [
    ...(entry.urlVarName ? [{ key: "url", name: entry.urlVarName }] : []),
    ...(entry.apikeyVarName ? [{ key: "apikey", name: entry.apikeyVarName }] : []),
  ];

  return {
    providerName: provider.handle,
    ...(policies ? { configuration: { policies } } : {}),
    ...(environmentVariables.length > 0 ? { environmentVariables } : {}),
  };
}

export function buildModelConfig(
  llmProviders: LLMProviderFormEntry[],
  initialEnvironmentName: string | undefined,
): ModelConfigRequest[] | undefined {
  if (!llmProviders.length) return undefined;
  const configs = llmProviders.map((entry) => buildOneModelConfig(entry, initialEnvironmentName))
    .filter((c): c is ModelConfigRequest => c !== null);
  return configs.length > 0 ? configs : undefined;
}

function buildOneMCPConfig(
  entry: MCPProxyFormEntry,
  initialEnvironmentName: string | undefined,
): MCPConfigRequest | null {
  // The create-side config applies only to the component's initial environment.
  const proxy = initialEnvironmentName
    ? entry.selectedProxyByEnv[initialEnvironmentName]
    : null;
  if (!proxy) return null;

  // Only an API-key endpoint has an API key to name: OAuth mints credentials
  // server-side and an unsecured endpoint needs none. Submitting one anyway
  // creates an env var row the platform injects permanently empty, with no error
  // at create or deploy (issue #1597).
  const apikeyVarName = mcpEntryHasAPIKeyVar(entry) ? entry.apikeyVarName : undefined;

  const environmentVariables = [
    ...(entry.urlVarName ? [{ key: "url", name: entry.urlVarName }] : []),
    ...(apikeyVarName ? [{ key: "apikey", name: apikeyVarName }] : []),
  ];

  return {
    proxyName: proxy.id,
    ...(environmentVariables.length > 0 ? { environmentVariables } : {}),
  };
}

export function buildMCPConfig(
  mcpProxies: MCPProxyFormEntry[],
  initialEnvironmentName: string | undefined,
): MCPConfigRequest[] | undefined {
  if (!mcpProxies.length) return undefined;
  const configs = mcpProxies.map((entry) => buildOneMCPConfig(entry, initialEnvironmentName))
    .filter((c): c is MCPConfigRequest => c !== null);
  return configs.length > 0 ? configs : undefined;
}

const subTypeByInterface = {
  CUSTOM: "custom-api",
  A2A: "a2a-agent",
  DEFAULT: "chat-api",
} as const;

export const buildAgentCreationPayload = (
  data: AddAgentFormValues,
  params: OrgProjPathParams,
  llmProviders: LLMProviderFormEntry[] = [],
  initialEnvironmentName?: string,
  mcpProxies: MCPProxyFormEntry[] = [],
): { params: OrgProjPathParams; body: CreateAgentRequest } => {
  const modelConfig = buildModelConfig(llmProviders, initialEnvironmentName);
  const mcpConfig = buildMCPConfig(mcpProxies, initialEnvironmentName);

  const labels =
    data.labels && Object.keys(data.labels).length > 0 ? data.labels : undefined;

  if (data.deploymentType === "new") {
    return {
      params,
      body: {
        name: data.name,
        displayName: data.displayName,
        description: data.description?.trim() || undefined,
        ...(labels ? { labels } : {}),
        provisioning: {
          type: "internal",
          repository: {
            url: data.repositoryUrl ?? "",
            branch: data.branch ?? "main",
            appPath: data.appPath?.trim() || "/",
            secretRef: data.gitSecretRef || null,
          },
        },
        // Three subtypes now, so a two-way ternary would silently map A2A to
        // chat. An A2A agent needs a port and no schema: it serves its own
        // agent card and the platform stores no copy.
        agentType: {
          type: "agent-api",
          subType: subTypeByInterface[data.interfaceType],
        },
        build: data.language === "docker"
          ? {
            type: "docker" as const,
            docker: {
              dockerfilePath: data.dockerfilePath ?? "./Dockerfile",
            },
          }
          : data.language === "ballerina"
          // Ballerina resolves its distribution version from Ballerina.toml, so no
          // language version or start command is collected in the UI.
          ? {
            type: "buildpack" as const,
            buildpack: {
              language: "ballerina",
            },
          }
          : {
            type: "buildpack" as const,
            buildpack: {
              language: data.language ?? "python",
              languageVersion: data.languageVersion ?? "3.11",
              runCommand: data.runCommand ?? "",
            },
          },
        configurations: {
          env: data.env
            .filter((envVar) => envVar.key && envVar.value)
            .map((envVar) => ({
              key: envVar.key!.replace(/\s+/g, '_'),
              value: envVar.value!,
              isSensitive: envVar.isSensitive || false,
            })),
          files: (data.files ?? [])
            .filter((f) => f.key && f.mountPath)
            .map((f) => ({
              key: f.key!,
              mountPath: f.mountPath!,
              value: f.value ?? '',
              isSensitive: f.isSensitive || false,
            })),
          enableAutoInstrumentation: data.enableAutoInstrumentation,
          ...(data.language === "python" &&
          data.enableAutoInstrumentation !== false &&
          data.instrumentationVersion
            ? { instrumentationVersion: data.instrumentationVersion }
            : {}),
        },
        inputInterface: {
          type: "HTTP",
          ...(data.interfaceType === "CUSTOM"
            ? {
              port: Number(data.port),
              basePath: data.basePath || "/",
              schema: {
                path: data.openApiPath ?? "",
              },
            }
            : data.interfaceType === "A2A"
            ? { port: Number(data.port) }
            : {}),
        },
        ...(modelConfig ? { modelConfig } : {}),
        ...(mcpConfig ? { mcpConfig } : {}),
      },
    };
  }

  return {
    params,
    body: {
      name: data.name,
      displayName: data.displayName,
      description: data.description,
      ...(labels ? { labels } : {}),
      provisioning: {
        type: "external",
      },
      agentType: {
        type: "external-agent-api",
        subType: "custom-api",
      },
      ...(modelConfig ? { modelConfig } : {}),
    },
  };
};

export const buildCatalogAgentPayload = (
  data: CreateAgentFormValues,
  params: OrgProjPathParams,
  kindName: string,
  version: string,
  llmProviders: LLMProviderFormEntry[] = [],
  initialEnvironmentName?: string,
  mcpProxies: MCPProxyFormEntry[] = [],
): { params: OrgProjPathParams; body: CreateAgentRequest } => {
  const modelConfig = buildModelConfig(llmProviders, initialEnvironmentName);
  const mcpConfig = buildMCPConfig(mcpProxies, initialEnvironmentName);

  return {
    params,
    body: {
      name: data.name,
      displayName: data.displayName,
      description: data.description?.trim() || undefined,
      ...(data.labels && Object.keys(data.labels).length > 0
        ? { labels: data.labels }
        : {}),
      provisioning: {
        type: "internal",
        agentKind: {
          name: kindName,
          version,
        },
      },
      configurations: {
        // A whitespace-only value (e.g. a pasted secret with a trailing newline) must
        // be treated the same as "left empty" — otherwise it both ships a broken,
        // barely-visible value AND skips the server-side default backfill for secret
        // fields, which only fires when the submitted value is truly empty.
        env: (data.env ?? [])
          .filter((envVar) => envVar.key && envVar.value?.trim())
          .map((envVar) => ({
            key: envVar.key!.trim().replace(/\s+/g, '_'),
            value: envVar.value!.trim(),
            isSensitive: envVar.isSensitive || false,
          })),
        files: (data.files ?? [])
          .filter((f) => f.key && f.mountPath)
          .map((f) => ({
            key: f.key!,
            mountPath: f.mountPath!,
            value: f.value ?? '',
            isSensitive: f.isSensitive || false,
          })),
        enableAutoInstrumentation: data.enableAutoInstrumentation,
      },
      ...(modelConfig ? { modelConfig } : {}),
      ...(mcpConfig ? { mcpConfig } : {}),
    },
  };
};

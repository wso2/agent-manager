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
  CreateAgentRequest,
  ModelConfigRequest,
  OrgProjPathParams,
} from "@agent-management-platform/types";
import { AddAgentFormValues, LLMProviderFormEntry } from "../form/schema";

function buildOneModelConfig(
  entry: LLMProviderFormEntry,
): ModelConfigRequest | null {
  const envMappings: ModelConfigRequest["envMappings"] = {};

  for (const [envName, provider] of Object.entries(entry.selectedProviderByEnv)) {
    if (!provider) continue;
    envMappings[envName] = {
      providerName: provider.handle,
      configuration: {
        policies:
          entry.guardrails.length > 0
            ? entry.guardrails.map((g) => ({
              name: g.name,
              version: g.version,
              paths: [{ path: "/*", methods: ["*"], params: g.settings ?? {} }],
            }))
            : undefined,
      },
    };
  }

  if (Object.keys(envMappings).length === 0) return null;

  const environmentVariables = [
    ...(entry.urlVarName ? [{ key: "url", name: entry.urlVarName }] : []),
    ...(entry.apikeyVarName ? [{ key: "apikey", name: entry.apikeyVarName }] : []),
  ];

  return {
    envMappings,
    ...(environmentVariables.length > 0 ? { environmentVariables } : {}),
  };
}

function buildModelConfig(
  llmProviders: LLMProviderFormEntry[],
): ModelConfigRequest[] | undefined {
  if (!llmProviders.length) return undefined;
  const configs = llmProviders.map(buildOneModelConfig)
    .filter((c): c is ModelConfigRequest => c !== null);
  return configs.length > 0 ? configs : undefined;
}

export const buildAgentCreationPayload = (
  data: AddAgentFormValues,
  params: OrgProjPathParams,
  llmProviders: LLMProviderFormEntry[] = [],
): { params: OrgProjPathParams; body: CreateAgentRequest } => {
  if (data.deploymentType === "new") {
    return {
      params,
      body: {
        name: data.name,
        displayName: data.displayName,
        description: data.description?.trim() || undefined,
        provisioning: {
          type: "internal",
          repository: {
            url: data.repositoryUrl ?? "",
            branch: data.branch ?? "main",
            appPath: data.appPath?.trim() || "/",
            secretRef: data.gitSecretRef || null,
          },
        },
        agentType: {
          type: "agent-api",
          subType: data.interfaceType === "CUSTOM" ? "custom-api" : "chat-api",
        },
        build: data.language === "docker"
          ? {
            type: "docker" as const,
            docker: {
              dockerfilePath: data.dockerfilePath ?? "./Dockerfile",
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
          enableAutoInstrumentation: data.enableAutoInstrumentation,
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
            : {}),
        },
        ...((buildModelConfig(llmProviders)) ?
          { modelConfig: buildModelConfig(llmProviders) } : {}),
      },
    };
  }

  return {
    params,
    body: {
      name: data.name,
      displayName: data.displayName,
      description: data.description,
      provisioning: {
        type: "external",
      },
      agentType: {
        type: "external-agent-api",
        subType: "custom-api",
      },
      ...((buildModelConfig(llmProviders)) ? { modelConfig: buildModelConfig(llmProviders) } : {}),
    },
  };
};

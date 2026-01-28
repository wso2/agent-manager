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
  OrgProjPathParams,
} from "@agent-management-platform/types";
import { AddAgentFormValues } from "../form/schema";

export const buildAgentCreationPayload = (
  data: AddAgentFormValues,
  params: OrgProjPathParams
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
          },
        },
        agentType: {
          type: "agent-api",
          subType: data.interfaceType === "CUSTOM" ? "custom-api" : "chat-api",
        },
        runtimeConfigs: {
          language: data.language ?? "python",
          languageVersion: data.languageVersion ?? "3.11",
          runCommand: data.runCommand ?? "",
          env: data.env
            .filter((envVar) => envVar.key && envVar.value)
            .map((envVar) => ({ key: envVar.key!, value: envVar.value! })),
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
        type: "agent-api",
        subType: "custom-api",
      },
    },
  };
};

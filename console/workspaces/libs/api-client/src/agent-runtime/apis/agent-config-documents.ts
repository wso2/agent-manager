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

import type {
  AgentConfigDocument,
  AgentConfigDocumentList,
  AgentConfigKind,
  AgentModelConfigListResponse,
  AgentModelConfigResponse,
  CreateAgentModelConfigRequest,
  EnvModelConfigRequest,
  ProviderConfig,
  UpdateAgentModelConfigRequest,
} from "@agent-management-platform/types";

// The only environment a binding lives in.
export const DEFAULT_ENVIRONMENT = "default";

const API_VERSION = "ai-control-plane.choreolabs.com/v1";

// targetOf is the provider or MCP proxy a mapping binds, whichever key the caller used.
function targetOf(m?: EnvModelConfigRequest): string | undefined {
  return m?.providerName ?? m?.proxyName ?? m?.proxyId ?? m?.mcpProxyName ?? m?.mcpProxyId;
}

// defaultMapping is the default environment's mapping, or the only one given.
function defaultMapping<M>(mappings?: Record<string, M>): M | undefined {
  if (!mappings) return undefined;
  return mappings[DEFAULT_ENVIRONMENT] ?? Object.values(mappings)[0];
}

function isMcp(kind: AgentConfigKind): boolean {
  return kind === "AgentMcpConfig";
}

// withTarget puts the bound resource under the key the kind uses.
function withTarget(kind: AgentConfigKind, target: string | undefined) {
  return isMcp(kind) ? { proxyName: target } : { providerName: target };
}

// agentConfigDocumentFromCreateRequest turns a create request into the document a POST takes.
export function agentConfigDocumentFromCreateRequest(
  kind: AgentConfigKind,
  project: string,
  body: CreateAgentModelConfigRequest,
): AgentConfigDocument {
  const mapping = defaultMapping(body.envMappings);
  return {
    apiVersion: API_VERSION,
    kind,
    metadata: { name: body.name, project },
    spec: {
      description: body.description,
      ...withTarget(kind, targetOf(mapping)),
      configuration: mapping?.configuration,
      environmentVariables: body.environmentVariables,
    },
  };
}

// agentConfigDocumentFromUpdateRequest applies an update request to the current document.
export function agentConfigDocumentFromUpdateRequest(
  kind: AgentConfigKind,
  project: string,
  body: UpdateAgentModelConfigRequest,
  current: AgentConfigDocument,
): AgentConfigDocument {
  const mapping = defaultMapping(body.envMappings);
  const spec = { ...current.spec };
  if (body.description !== undefined) spec.description = body.description;
  if (mapping) {
    const bound = isMcp(kind) ? spec.proxyName : spec.providerName;
    Object.assign(spec, withTarget(kind, targetOf(mapping) ?? bound));
    spec.configuration = mapping.configuration;
  }
  if (body.environmentVariables !== undefined) {
    spec.environmentVariables = body.environmentVariables;
  }
  return {
    apiVersion: API_VERSION,
    kind,
    metadata: { name: body.name ?? current.metadata.name, project },
    spec,
  };
}

// agentConfigFromDocument renders a document in the shape the pages read.
export function agentConfigFromDocument(
  doc: AgentConfigDocument,
  agentName: string,
): AgentModelConfigResponse {
  const mcp = isMcp(doc.kind);
  const target = mcp ? doc.spec.proxyName : doc.spec.providerName;
  const status = doc.status;
  const configuration: ProviderConfig = {
    providerName: target,
    providerUuid: status?.providerUuid,
    proxyUuid: status?.proxyUuid,
    proxyName: mcp ? target : status?.proxyName,
    url: status?.url,
    authInfo: status?.authInfo,
    policies: doc.spec.configuration?.policies,
    providerPolicies: status?.providerPolicies,
    resilience: doc.spec.configuration?.resilience,
    status: "active",
    ...(mcp ? { proxyId: target, mcpProxyName: target, mcpProxyId: target } : {}),
  };
  return {
    uuid: doc.metadata.uid ?? "",
    name: doc.metadata.name,
    description: doc.spec.description,
    agentId: agentName,
    type: mcp ? "mcp" : "llm",
    organizationName: "",
    projectName: doc.metadata.project,
    // A binding with nothing in the default environment has no mapping.
    envMappings: target || status
      ? { [DEFAULT_ENVIRONMENT]: { environmentName: DEFAULT_ENVIRONMENT, configuration } }
      : {},
    environmentVariables: doc.spec.environmentVariables ?? [],
    createdAt: doc.metadata.creationTimestamp ?? "",
    updatedAt: doc.metadata.updateTimestamp ?? "",
  };
}

// agentConfigListFromDocuments renders a page of documents as the list the pages read.
export function agentConfigListFromDocuments(
  page: AgentConfigDocumentList,
  agentName: string,
): AgentModelConfigListResponse {
  return {
    configs: page.list.map((doc) => {
      const c = agentConfigFromDocument(doc, agentName);
      return {
        uuid: c.uuid,
        name: c.name,
        description: c.description,
        agentId: c.agentId,
        type: c.type,
        organizationName: c.organizationName,
        projectName: c.projectName,
        createdAt: c.createdAt,
        updatedAt: c.updatedAt,
      };
    }),
    pagination: page.pagination,
  };
}

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type {
  CreateLLMAPIKeyRequest,
  CreateLLMAPIKeyResponse,
  CreateLLMProviderAPIKeyPathParams,
  CreateLLMProviderPathParams,
  CreateLLMProviderRequest,
  CreateLLMProviderTemplatePathParams,
  CreateLLMProviderTemplateRequest,
  CreateLLMProxyAPIKeyPathParams,
  CreateLLMProxyPathParams,
  CreateLLMProxyRequest,
  DeleteLLMProviderPathParams,
  DeleteLLMProviderTemplatePathParams,
  DeleteLLMProxyPathParams,
  GetLLMProviderPathParams,
  GetLLMProviderTemplatePathParams,
  GetLLMProxyPathParams,
  ListAvailableLLMPoliciesPathParams,
  ListLLMDeploymentsPathParams,
  ListAPIKeysResponse,
  ListLLMProviderAPIKeysPathParams,
  ListLLMProxyAPIKeysPathParams,
  ListLLMProviderConsumersPathParams,
  ListLLMProviderTemplatesPathParams,
  ListLLMProvidersPathParams,
  ListLLMProxiesPathParams,
  LLMDeploymentListResponse,
  LLMDeploymentResponse,
  LLMPolicyAvailabilityResponse,
  LLMProviderConsumerListResponse,
  LLMProviderDocument,
  LLMProviderDocumentList,
  LLMProviderUpdateSpec,
  LLMProviderListResponse,
  LLMProviderResponse,
  LLMProviderTemplateDocument,
  LLMProviderTemplateDocumentList,
  LLMProviderTemplateListResponse,
  LLMProviderTemplateResponse,
  LLMProxyListResponse,
  LLMProxyResponse,
  RestoreLLMDeploymentPathParams,
  RestoreLLMDeploymentQuery,
  RevokeLLMProviderAPIKeyPathParams,
  RevokeLLMProxyAPIKeyPathParams,
  RotateLLMAPIKeyRequest,
  RotateLLMAPIKeyResponse,
  RotateLLMProviderAPIKeyPathParams,
  RotateLLMProxyAPIKeyPathParams,
  UpdateLLMProviderPathParams,
  UpdateLLMProviderRequest,
  UpdateLLMProviderTemplatePathParams,
  UpdateLLMProxyPathParams,
  UpdateLLMProxyRequest,
} from "@agent-management-platform/types";
import { encodeRequired, httpDELETE, httpGET, httpPOST, httpPUT, GATEWAY_BASE } from "../../shared/utils";

const PROVIDERS = `${GATEWAY_BASE}/llm-providers`;
const TEMPLATES = `${GATEWAY_BASE}/llm-provider-templates`;
const API_VERSION = "ai-control-plane.choreolabs.com/v1";
const KIND = "LlmProvider";

interface PaginationQuery {
  limit?: number;
  offset?: number;
}

export async function listLLMProviderTemplates(
  _params: ListLLMProviderTemplatesPathParams,
  query?: PaginationQuery,
  getToken?: () => Promise<string>,
): Promise<LLMProviderTemplateDocumentList> {
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(
    `${TEMPLATES}`,
    {
      token,
      searchParams: Object.keys(searchParams).length ? searchParams : undefined,
    },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMProviderTemplate(
  _params: CreateLLMProviderTemplatePathParams,
  body: LLMProviderTemplateDocument,
  getToken?: () => Promise<string>,
): Promise<LLMProviderTemplateDocument> {
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${TEMPLATES}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getLLMProviderTemplate(
  params: GetLLMProviderTemplatePathParams,
  getToken?: () => Promise<string>,
): Promise<LLMProviderTemplateDocument> {
  const id = encodeRequired(params.templateId, "templateId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${TEMPLATES}/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateLLMProviderTemplate(
  params: UpdateLLMProviderTemplatePathParams,
  body: LLMProviderTemplateDocument,
  getToken?: () => Promise<string>,
): Promise<LLMProviderTemplateDocument> {
  const id = encodeRequired(params.templateId, "templateId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${TEMPLATES}/${id}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteLLMProviderTemplate(
  params: DeleteLLMProviderTemplatePathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const id = encodeRequired(params.templateId, "templateId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${TEMPLATES}/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export async function listLLMProviders(
  _params: ListLLMProvidersPathParams,
  query?: PaginationQuery,
  getToken?: () => Promise<string>,
): Promise<LLMProviderDocumentList> {
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(PROVIDERS, {
    token,
    searchParams: Object.keys(searchParams).length ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listAvailableLLMPolicies(
  _params: ListAvailableLLMPoliciesPathParams,
  query?: { providerId?: string },
  getToken?: () => Promise<string>,
): Promise<LLMPolicyAvailabilityResponse> {
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.providerId) {
    searchParams.providerId = query.providerId;
  }

  // httpGET already throws on a non-OK response, so no !res.ok branch is needed.
  const res = await httpGET(`${PROVIDERS}/policies`, {
    token,
    searchParams: Object.keys(searchParams).length ? searchParams : undefined,
  });
  return res.json();
}

export async function createLLMProvider(
  _params: CreateLLMProviderPathParams,
  body: LLMProviderDocument,
  getToken?: () => Promise<string>,
): Promise<LLMProviderDocument> {
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(PROVIDERS, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getLLMProvider(
  params: GetLLMProviderPathParams,
  getToken?: () => Promise<string>,
): Promise<LLMProviderDocument> {
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${PROVIDERS}/${id}`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateLLMProvider(
  params: UpdateLLMProviderPathParams,
  body: LLMProviderDocument<LLMProviderUpdateSpec>,
  getToken?: () => Promise<string>,
): Promise<LLMProviderDocument> {
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(`${PROVIDERS}/${id}`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteLLMProvider(
  params: DeleteLLMProviderPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(`${PROVIDERS}/${id}`, { token });
  if (!res.ok) throw await res.json();
}

// The UI works on a flat provider; these convert it to and from the document the API serves.

export function providerFromDocument(doc: LLMProviderDocument): LLMProviderResponse {
  const { metadata, spec } = doc;
  return {
    uuid: metadata.uid ?? "",
    id: metadata.name,
    name: spec.displayName,
    version: spec.version,
    context: spec.context,
    template: spec.template,
    upstream: spec.upstream,
    resilience: spec.resilience,
    description: spec.description,
    createdBy: metadata.createdBy,
    accessControl: spec.accessControl,
    policies: spec.policies,
    openapi: spec.openapi,
    modelProviders: spec.modelProviders,
    rateLimiting: spec.rateLimiting,
    security: spec.security,
    gateways: spec.gateways,
    inCatalog: spec.inCatalog,
    createdAt: metadata.creationTimestamp,
    updatedAt: metadata.updateTimestamp,
  };
}

export function providerListFromDocuments(list: LLMProviderDocumentList): LLMProviderListResponse {
  const providers = list.list.map(providerFromDocument);
  return {
    providers,
    total: list.pagination?.total ?? providers.length,
    limit: list.pagination?.limit ?? providers.length,
    offset: list.pagination?.offset ?? 0,
  };
}

export function documentFromCreateRequest(body: CreateLLMProviderRequest): LLMProviderDocument {
  const { id, name, ...spec } = body;
  return {
    apiVersion: API_VERSION,
    kind: KIND,
    metadata: { name: id },
    spec: { ...spec, displayName: name },
  };
}

// Leaves catalog membership out so an edit never changes it; absent fields keep stored values.
export function documentFromUpdateRequest(
  handle: string,
  body: UpdateLLMProviderRequest,
): LLMProviderDocument<LLMProviderUpdateSpec> {
  const { name, ...rest } = body;
  const spec: LLMProviderUpdateSpec = {
    displayName: name ?? "",
    description: rest.description,
    version: rest.version,
    context: rest.context,
    template: rest.template,
    upstream: rest.upstream,
    resilience: rest.resilience,
    accessControl: rest.accessControl,
    policies: rest.policies,
    openapi: rest.openapi,
    modelProviders: rest.modelProviders,
    rateLimiting: rest.rateLimiting,
    security: rest.security,
    gateways: rest.gateways,
  };
  return {
    apiVersion: API_VERSION,
    kind: KIND,
    metadata: { name: handle },
    spec,
  };
}

// The UI works on a flat template; these convert it to and from the document the API serves.

export function templateFromDocument(
  doc: LLMProviderTemplateDocument,
): LLMProviderTemplateResponse {
  const { metadata, spec } = doc;
  const { displayName, ...rest } = spec;
  return {
    ...rest,
    uuid: metadata.uid ?? "",
    id: metadata.name,
    name: displayName,
    createdBy: metadata.createdBy,
    createdAt: metadata.creationTimestamp ?? "",
    updatedAt: metadata.updateTimestamp ?? "",
  };
}

export function templateListFromDocuments(
  list: LLMProviderTemplateDocumentList,
): LLMProviderTemplateListResponse {
  const templates = list.list.map(templateFromDocument);
  return {
    templates,
    total: list.pagination?.total ?? templates.length,
    limit: list.pagination?.limit ?? templates.length,
    offset: list.pagination?.offset ?? 0,
  };
}

export function templateDocumentFromRequest(
  body: CreateLLMProviderTemplateRequest,
): LLMProviderTemplateDocument {
  const { id, name, ...spec } = body;
  return {
    apiVersion: API_VERSION,
    kind: "LlmProviderTemplate",
    metadata: { name: id },
    spec: { ...spec, displayName: name },
  };
}

export async function listLLMProxies(
  params: ListLLMProxiesPathParams,
  query?: PaginationQuery,
  getToken?: () => Promise<string>,
): Promise<LLMProxyListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies`,
    {
      token,
      searchParams: Object.keys(searchParams).length ? searchParams : undefined,
    },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMProxy(
  params: CreateLLMProxyPathParams,
  body: CreateLLMProxyRequest,
  getToken?: () => Promise<string>,
): Promise<LLMProxyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getLLMProxy(
  params: GetLLMProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<LLMProxyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const id = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateLLMProxy(
  params: UpdateLLMProxyPathParams,
  body: UpdateLLMProxyRequest,
  getToken?: () => Promise<string>,
): Promise<LLMProxyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const id = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteLLMProxy(
  params: DeleteLLMProxyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const id = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export async function listLLMDeployments(
  params: ListLLMDeploymentsPathParams,
  query?: { gatewayId?: string; status?: string },
  getToken?: () => Promise<string>,
): Promise<LLMDeploymentListResponse> {
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.gatewayId !== undefined) {
    searchParams.gatewayId = query.gatewayId;
  }
  if (query?.status !== undefined) {
    searchParams.status = query.status;
  }

  const res = await httpGET(
    `${PROVIDERS}/${id}/deployments`,
    {
      token,
      searchParams: Object.keys(searchParams).length ? searchParams : undefined,
    },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function restoreLLMDeployment(
  params: RestoreLLMDeploymentPathParams,
  query: RestoreLLMDeploymentQuery,
  getToken?: () => Promise<string>,
): Promise<LLMDeploymentResponse> {
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    deploymentId: query.deploymentId,
    gatewayId: query.gatewayId,
  };

  const res = await httpPOST(
    `${PROVIDERS}/${id}/deployments/restore`,
    {},
    { token, searchParams },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

// -----------------------------------------------------------------------------
// LLM API keys — provider
// -----------------------------------------------------------------------------

export async function listLLMProviderAPIKeys(
  params: ListLLMProviderAPIKeysPathParams,
  getToken?: () => Promise<string>,
): Promise<ListAPIKeysResponse> {
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${PROVIDERS}/${id}/api-keys`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMProviderAPIKey(
  params: CreateLLMProviderAPIKeyPathParams,
  body: CreateLLMAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateLLMAPIKeyResponse> {
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${PROVIDERS}/${id}/api-keys`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rotateLLMProviderAPIKey(
  params: RotateLLMProviderAPIKeyPathParams,
  body: RotateLLMAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<RotateLLMAPIKeyResponse> {
  const id = encodeRequired(params.providerId, "providerId");
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${PROVIDERS}/${id}/api-keys/${keyName}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeLLMProviderAPIKey(
  params: RevokeLLMProviderAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const id = encodeRequired(params.providerId, "providerId");
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${PROVIDERS}/${id}/api-keys/${keyName}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

// -----------------------------------------------------------------------------
// LLM API keys — proxy
// -----------------------------------------------------------------------------

export async function listLLMProxyAPIKeys(
  params: ListLLMProxyAPIKeysPathParams,
  getToken?: () => Promise<string>,
): Promise<ListAPIKeysResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const id = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}/api-keys`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMProxyAPIKey(
  params: CreateLLMProxyAPIKeyPathParams,
  body: CreateLLMAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateLLMAPIKeyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const id = encodeRequired(params.proxyId, "proxyId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}/api-keys`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rotateLLMProxyAPIKey(
  params: RotateLLMProxyAPIKeyPathParams,
  body: RotateLLMAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<RotateLLMAPIKeyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const id = encodeRequired(params.proxyId, "proxyId");
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}/api-keys/${keyName}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeLLMProxyAPIKey(
  params: RevokeLLMProxyAPIKeyPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const proj = encodeRequired(params.projName, "projName");
  const id = encodeRequired(params.proxyId, "proxyId");
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${GATEWAY_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}/api-keys/${keyName}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export async function listLLMProviderConsumers(
  params: ListLLMProviderConsumersPathParams,
  getToken?: () => Promise<string>,
): Promise<LLMProviderConsumerListResponse> {
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${PROVIDERS}/${id}/consumers`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

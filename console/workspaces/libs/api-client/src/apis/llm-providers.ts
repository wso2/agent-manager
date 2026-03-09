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
  CreateLLMDeploymentPathParams,
  DeployLLMProviderRequest,
  CreateLLMProviderAPIKeyPathParams,
  CreateLLMProviderPathParams,
  CreateLLMProviderRequest,
  CreateLLMProviderTemplatePathParams,
  CreateLLMProviderTemplateRequest,
  CreateLLMProxyAPIKeyPathParams,
  CreateLLMProxyPathParams,
  CreateLLMProxyRequest,
  DeleteLLMDeploymentPathParams,
  DeleteLLMProviderPathParams,
  DeleteLLMProviderTemplatePathParams,
  DeleteLLMProxyPathParams,
  GetLLMDeploymentPathParams,
  GetLLMProviderPathParams,
  GetLLMProviderTemplatePathParams,
  GetLLMProxyPathParams,
  ListLLMDeploymentsPathParams,
  ListLLMProviderProxiesPathParams,
  ListLLMProviderTemplatesPathParams,
  ListLLMProvidersPathParams,
  ListLLMProxiesPathParams,
  LLMDeploymentListResponse,
  LLMDeploymentResponse,
  LLMProviderListResponse,
  LLMProviderResponse,
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
  UndeployLLMProviderPathParams,
  UndeployLLMProviderQuery,
  UpdateLLMProviderCatalogPathParams,
  UpdateLLMProviderCatalogRequest,
  UpdateLLMProviderPathParams,
  UpdateLLMProviderRequest,
  UpdateLLMProviderTemplatePathParams,
  UpdateLLMProviderTemplateRequest,
  UpdateLLMProxyPathParams,
  UpdateLLMProxyRequest,
} from "@agent-management-platform/types/dist/api/llm-providers";
import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";

interface PaginationQuery {
  limit?: number;
  offset?: number;
}

export async function listLLMProviderTemplates(
  params: ListLLMProviderTemplatesPathParams,
  query?: PaginationQuery,
  getToken?: () => Promise<string>,
): Promise<LLMProviderTemplateListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/llm-provider-templates`,
    {
      token,
      searchParams: Object.keys(searchParams).length ? searchParams : undefined,
    },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMProviderTemplate(
  params: CreateLLMProviderTemplatePathParams,
  body: CreateLLMProviderTemplateRequest,
  getToken?: () => Promise<string>,
): Promise<LLMProviderTemplateResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/llm-provider-templates`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getLLMProviderTemplate(
  params: GetLLMProviderTemplatePathParams,
  getToken?: () => Promise<string>,
): Promise<LLMProviderTemplateResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.templateId, "templateId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/llm-provider-templates/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateLLMProviderTemplate(
  params: UpdateLLMProviderTemplatePathParams,
  body: UpdateLLMProviderTemplateRequest,
  getToken?: () => Promise<string>,
): Promise<LLMProviderTemplateResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.templateId, "templateId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/llm-provider-templates/${id}`,
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
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.templateId, "templateId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/llm-provider-templates/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export async function listLLMProviders(
  params: ListLLMProvidersPathParams,
  query?: PaginationQuery,
  getToken?: () => Promise<string>,
): Promise<LLMProviderListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/llm-providers`, {
    token,
    searchParams: Object.keys(searchParams).length ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMProvider(
  params: CreateLLMProviderPathParams,
  body: CreateLLMProviderRequest,
  getToken?: () => Promise<string>,
): Promise<LLMProviderResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(`${SERVICE_BASE}/orgs/${org}/llm-providers`, body, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getLLMProvider(
  params: GetLLMProviderPathParams,
  getToken?: () => Promise<string>,
): Promise<LLMProviderResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/llm-providers/${id}`, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateLLMProvider(
  params: UpdateLLMProviderPathParams,
  body: UpdateLLMProviderRequest,
  getToken?: () => Promise<string>,
): Promise<LLMProviderResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteLLMProvider(
  params: DeleteLLMProviderPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(`${SERVICE_BASE}/orgs/${org}/llm-providers/${id}`, {
    token,
  });
  if (!res.ok) throw await res.json();
}

export async function updateLLMProviderCatalog(
  params: UpdateLLMProviderCatalogPathParams,
  body: UpdateLLMProviderCatalogRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/catalog`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export async function listLLMProviderProxies(
  params: ListLLMProviderProxiesPathParams,
  query?: PaginationQuery,
  getToken?: () => Promise<string>,
): Promise<LLMProxyListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/llm-proxies`,
    {
      token,
      searchParams: Object.keys(searchParams).length ? searchParams : undefined,
    },
  );
  if (!res.ok) throw await res.json();
  return res.json();
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
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/llm-proxies`,
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
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/llm-proxies`,
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
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}`,
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
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}`,
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
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export async function listLLMDeployments(
  params: ListLLMDeploymentsPathParams,
  query?: { gatewayId?: string; status?: string },
  getToken?: () => Promise<string>,
): Promise<LLMDeploymentListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
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
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/deployments`,
    {
      token,
      searchParams: Object.keys(searchParams).length ? searchParams : undefined,
    },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createLLMDeployment(
  params: CreateLLMDeploymentPathParams,
  body: DeployLLMProviderRequest,
  getToken?: () => Promise<string>,
): Promise<LLMDeploymentResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/deployments`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function undeployLLMProvider(
  params: UndeployLLMProviderPathParams,
  query: UndeployLLMProviderQuery,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    deploymentId: query.deploymentId,
    gatewayId: query.gatewayId,
  };

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/deployments/undeploy`,
    {},
    { token, searchParams },
  );
  if (!res.ok) throw await res.json();
}

export async function restoreLLMDeployment(
  params: RestoreLLMDeploymentPathParams,
  query: RestoreLLMDeploymentQuery,
  getToken?: () => Promise<string>,
): Promise<LLMDeploymentResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {
    deploymentId: query.deploymentId,
    gatewayId: query.gatewayId,
  };

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/deployments/restore`,
    {},
    { token, searchParams },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getLLMDeployment(
  params: GetLLMDeploymentPathParams,
  getToken?: () => Promise<string>,
): Promise<LLMDeploymentResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const deploymentId = encodeRequired(params.deploymentId, "deploymentId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/deployments/${deploymentId}`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteLLMDeployment(
  params: DeleteLLMDeploymentPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const deploymentId = encodeRequired(params.deploymentId, "deploymentId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/deployments/${deploymentId}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

// -----------------------------------------------------------------------------
// LLM API keys — provider
// -----------------------------------------------------------------------------

export async function createLLMProviderAPIKey(
  params: CreateLLMProviderAPIKeyPathParams,
  body: CreateLLMAPIKeyRequest,
  getToken?: () => Promise<string>,
): Promise<CreateLLMAPIKeyResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/api-keys`,
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
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/api-keys/${keyName}`,
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
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.providerId, "providerId");
  const keyName = encodeRequired(params.keyName, "keyName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/llm-providers/${id}/api-keys/${keyName}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

// -----------------------------------------------------------------------------
// LLM API keys — proxy
// -----------------------------------------------------------------------------

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
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}/api-keys`,
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
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}/api-keys/${keyName}`,
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
    `${SERVICE_BASE}/orgs/${org}/projects/${proj}/llm-proxies/${id}/api-keys/${keyName}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

function encodeRequired(value: string | undefined, label: string): string {
  if (!value) {
    throw new Error(`Missing required parameter: ${label}`);
  }
  return encodeURIComponent(value);
}

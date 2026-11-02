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
  CreateGatewayPathParams,
  CreateGatewayRequest,
  DeleteGatewayPathParams,
  DiscoverOidcPathParams,
  DiscoverOidcQuery,
  GetGatewayPathParams,
  IdentityProviderListResponse,
  OidcDiscoveryResponse,
  ListEnvironmentIdentityProvidersPathParams,
  ListGatewaysPathParams,
  ListGatewaysQuery,
  ListIdentityProvidersPathParams,
  UpsertIdentityProviderPathParams,
  UpsertIdentityProviderRequest,
  DeleteIdentityProviderPathParams,
  IdentityProvider,
  GatewayListResponse,
  GatewayResponse,
  GatewayTokenListResponse,
  GatewayTokenResponse,
  UpdateGatewayPathParams,
  UpdateGatewayRequest,
  CreateSelfHostedGatewayPathParams,
  DeleteSelfHostedGatewayPathParams,
  ListSelfHostedGatewaysPathParams,
  ListSelfHostedGatewaysQuery,
  RevokeSelfHostedGatewayTokenPathParams,
  SelfHostedGatewayRequest,
  SelfHostedGatewayTokensPathParams,
  SelfHostedGatewayUpdateRequest,
  UpdateSelfHostedGatewayPathParams,
} from "@agent-management-platform/types";
import {
  encodeRequired,
  httpDELETE,
  httpGET,
  httpPOST,
  httpPUT,
  SERVICE_BASE,
} from "../utils";

export async function listGateways(
  params: ListGatewaysPathParams,
  query?: ListGatewaysQuery,
  getToken?: () => Promise<string>,
): Promise<GatewayListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }
  if (query?.type) {
    searchParams.type = query.type;
  }
  if (query?.status) {
    searchParams.status = query.status;
  }
  if (query?.environment) {
    searchParams.environment = query.environment;
  }

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/gateways`, {
    token,
    searchParams: Object.keys(searchParams).length ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createGateway(
  params: CreateGatewayPathParams,
  body: CreateGatewayRequest,
  getToken?: () => Promise<string>,
): Promise<GatewayResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/gateways`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getGateway(
  params: GetGatewayPathParams,
  getToken?: () => Promise<string>,
): Promise<GatewayResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/gateways/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateGateway(
  params: UpdateGatewayPathParams,
  body: UpdateGatewayRequest,
  getToken?: () => Promise<string>,
): Promise<GatewayResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/gateways/${id}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteGateway(
  params: DeleteGatewayPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/gateways/${id}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export interface AssignGatewayToEnvironmentParams {
  orgName: string;
  gatewayId: string;
  envId: string;
}

export async function assignGatewayToEnvironment(
  params: AssignGatewayToEnvironmentParams,
  getToken?: () => Promise<string>,
): Promise<GatewayResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const gatewayId = encodeRequired(params.gatewayId, "gatewayId");
  const envId = encodeRequired(params.envId, "envId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/gateways/${gatewayId}/environments/${envId}`,
    {},
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export interface RemoveGatewayFromEnvironmentParams {
  orgName: string;
  gatewayId: string;
  envId: string;
}

export async function removeGatewayFromEnvironment(
  params: RemoveGatewayFromEnvironmentParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const gatewayId = encodeRequired(params.gatewayId, "gatewayId");
  const envId = encodeRequired(params.envId, "envId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/gateways/${gatewayId}/environments/${envId}`,
    { token },
  );
  if (!res.ok) throw await res.json();
}

export interface ListGatewayTokensParams {
  orgName: string;
  gatewayId: string;
}

export async function listGatewayTokens(
  params: ListGatewayTokensParams,
  getToken?: () => Promise<string>,
): Promise<GatewayTokenListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/gateways/${id}/tokens`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rotateGatewayToken(
  params: ListGatewayTokensParams,
  getToken?: () => Promise<string>,
): Promise<GatewayTokenResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/gateways/${id}/tokens`,
    {},
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export interface RevokeGatewayTokenParams {
  orgName: string;
  gatewayId: string;
  tokenId: string;
}

export async function revokeGatewayToken(
  params: RevokeGatewayTokenParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const tokenId = encodeRequired(params.tokenId, "tokenId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/gateways/${id}/tokens/${tokenId}`,
    { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function listIdentityProviders(
  params: ListIdentityProvidersPathParams,
  getToken?: () => Promise<string>,
): Promise<IdentityProviderListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/identity-providers`, {
    token,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function discoverOidc(
  params: DiscoverOidcPathParams,
  query: DiscoverOidcQuery,
  getToken?: () => Promise<string>,
): Promise<OidcDiscoveryResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/identity-providers/discover`, {
    token,
    searchParams: { url: query.url },
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listEnvironmentIdentityProviders(
  params: ListEnvironmentIdentityProvidersPathParams,
  getToken?: () => Promise<string>,
): Promise<IdentityProviderListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const envId = encodeRequired(params.environmentId, "environmentId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/environments/${envId}/identity-providers`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function upsertIdentityProvider(
  params: UpsertIdentityProviderPathParams,
  body: UpsertIdentityProviderRequest,
  getToken?: () => Promise<string>,
): Promise<IdentityProvider> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const name = encodeRequired(params.name, "name");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/gateways/${id}/identity-providers/${name}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteIdentityProvider(
  params: DeleteIdentityProviderPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const name = encodeRequired(params.name, "name");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/gateways/${id}/identity-providers/${name}`,
    { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function listSelfHostedGateways(
  params: ListSelfHostedGatewaysPathParams,
  query?: ListSelfHostedGatewaysQuery,
  getToken?: () => Promise<string>,
): Promise<GatewayListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (query?.limit !== undefined) {
    searchParams.limit = String(query.limit);
  }
  if (query?.offset !== undefined) {
    searchParams.offset = String(query.offset);
  }

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/self-hosted-gateways`, {
    token,
    searchParams: Object.keys(searchParams).length ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createSelfHostedGateway(
  params: CreateSelfHostedGatewayPathParams,
  body: SelfHostedGatewayRequest,
  getToken?: () => Promise<string>,
): Promise<GatewayResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/self-hosted-gateways`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateSelfHostedGateway(
  params: UpdateSelfHostedGatewayPathParams,
  body: SelfHostedGatewayUpdateRequest,
  getToken?: () => Promise<string>,
): Promise<GatewayResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(
    `${SERVICE_BASE}/orgs/${org}/self-hosted-gateways/${id}`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteSelfHostedGateway(
  params: DeleteSelfHostedGatewayPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/self-hosted-gateways/${id}`,
    { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function listSelfHostedGatewayTokens(
  params: SelfHostedGatewayTokensPathParams,
  getToken?: () => Promise<string>,
): Promise<GatewayTokenListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/self-hosted-gateways/${id}/tokens`,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

/** The token value is returned only in this response. */
export async function createSelfHostedGatewayToken(
  params: SelfHostedGatewayTokensPathParams,
  getToken?: () => Promise<string>,
): Promise<GatewayTokenResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/self-hosted-gateways/${id}/tokens`,
    {},
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function revokeSelfHostedGatewayToken(
  params: RevokeSelfHostedGatewayTokenPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const tokenId = encodeRequired(params.tokenId, "tokenId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${org}/self-hosted-gateways/${id}/tokens/${tokenId}`,
    { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

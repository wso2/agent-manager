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
  CreateSelfHostedGatewayPathParams,
  DeleteGatewayPathParams,
  DeleteIdentityProviderPathParams,
  DiscoverOidcPathParams,
  DiscoverOidcQuery,
  Environment,
  GatewayDocument,
  GatewayDocumentList,
  GatewayEnvironmentResponse,
  GatewayListResponse,
  GatewayResponse,
  GatewayTokenListResponse,
  GatewayTokenResponse,
  GetGatewayPathParams,
  IdentityProvider,
  IdentityProviderListResponse,
  ListGatewaysPathParams,
  ListGatewaysQuery,
  ListIdentityProvidersPathParams,
  OidcDiscoveryResponse,
  SelfHostedGatewayRequest,
  SelfHostedGatewayUpdateRequest,
  UpdateSelfHostedGatewayPathParams,
  UpsertIdentityProviderPathParams,
  UpsertIdentityProviderRequest,
} from "@agent-management-platform/types";
import { encodeRequired, httpDELETE, httpGET, httpPOST, httpPUT, GATEWAY_BASE } from "../../shared/utils";

const API_VERSION = "ai-control-plane.choreolabs.com/v1";
const GATEWAYS = `${GATEWAY_BASE}/gateways`;

/** A core environment in the shape a gateway response lists it. */
export function gatewayEnvironment(env: Environment): GatewayEnvironmentResponse {
  return {
    id: env.id ?? "",
    organizationName: "",
    name: env.name,
    displayName: env.displayName ?? env.name,
    dataplaneRef: env.dataplaneRef,
    dnsPrefix: env.dnsPrefix ?? "",
    isProduction: env.isProduction,
    createdAt: env.createdAt,
    updatedAt: env.createdAt,
  };
}

/** The flat gateway the pages use; every gateway serves the organization's environments. */
export function gatewayFromDocument(
  doc: GatewayDocument,
  environments: GatewayEnvironmentResponse[] = [],
): GatewayResponse {
  const { metadata, spec, status } = doc;
  return {
    uuid: metadata.uid ?? "",
    organizationName: "",
    name: metadata.name,
    displayName: spec.displayName,
    description: spec.description,
    gatewayType: spec.type ?? "EGRESS",
    hosting: spec.hosting ?? "MANAGED",
    vhost: spec.endpoint ?? "",
    runtimeUrl: spec.runtimeUrl,
    isCritical: spec.isCritical ?? false,
    properties: spec.properties,
    status: status?.state ?? "INACTIVE",
    createdAt: metadata.creationTimestamp ?? "",
    updatedAt: metadata.updateTimestamp ?? "",
    environments,
  };
}

export function gatewayListFromDocuments(
  list: GatewayDocumentList,
  environments: GatewayEnvironmentResponse[] = [],
): GatewayListResponse {
  const gateways = list.list.map((doc) => gatewayFromDocument(doc, environments));
  return {
    gateways,
    total: list.pagination?.total ?? gateways.length,
    limit: list.pagination?.limit ?? gateways.length,
    offset: list.pagination?.offset ?? 0,
  };
}

/** The document that registers a self-hosted gateway from the add-gateway form. */
export function selfHostedGatewayDocument(body: SelfHostedGatewayRequest): GatewayDocument {
  return {
    apiVersion: API_VERSION,
    kind: "Gateway",
    metadata: { name: body.handle },
    spec: {
      displayName: body.displayName,
      description: body.description,
      hosting: "SELF_HOSTED",
      endpoint: body.endpoints[0],
      isCritical: body.isCritical,
      properties: body.properties,
    },
  };
}

export async function listGateways(
  _params: ListGatewaysPathParams,
  query?: ListGatewaysQuery,
  getToken?: () => Promise<string>,
): Promise<GatewayDocumentList> {
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
  if (query?.hosting) {
    searchParams.hosting = query.hosting;
  }

  const res = await httpGET(GATEWAYS, {
    token,
    searchParams: Object.keys(searchParams).length ? searchParams : undefined,
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getGateway(
  params: GetGatewayPathParams,
  getToken?: () => Promise<string>,
): Promise<GatewayDocument> {
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${GATEWAYS}/${id}`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createSelfHostedGateway(
  _params: CreateSelfHostedGatewayPathParams,
  body: SelfHostedGatewayRequest,
  getToken?: () => Promise<string>,
): Promise<GatewayDocument> {
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(GATEWAYS, selfHostedGatewayDocument(body), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * A PUT replaces the gateway's editable fields, so the stored document is read and the changes
 * laid over it.
 */
export async function updateSelfHostedGateway(
  params: UpdateSelfHostedGatewayPathParams,
  body: SelfHostedGatewayUpdateRequest,
  getToken?: () => Promise<string>,
): Promise<GatewayDocument> {
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const current = await getGateway(params, getToken);
  const token = getToken ? await getToken() : undefined;

  const doc: GatewayDocument = {
    apiVersion: API_VERSION,
    kind: "Gateway",
    metadata: { name: current.metadata.name },
    spec: { ...current.spec, ...body, displayName: body.displayName ?? current.spec.displayName },
  };
  const res = await httpPUT(`${GATEWAYS}/${id}`, doc, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteGateway(
  params: DeleteGatewayPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(`${GATEWAYS}/${id}`, { token });
  if (!res.ok) throw await res.json();
}

export interface ListGatewayTokensParams {
  orgName: string | undefined;
  gatewayId: string | undefined;
}

export async function listGatewayTokens(
  params: ListGatewayTokensParams,
  getToken?: () => Promise<string>,
): Promise<GatewayTokenListResponse> {
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${GATEWAYS}/${id}/tokens`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rotateGatewayToken(
  params: ListGatewayTokensParams,
  getToken?: () => Promise<string>,
): Promise<GatewayTokenResponse> {
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(`${GATEWAYS}/${id}/tokens`, {}, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export interface RevokeGatewayTokenParams extends ListGatewayTokensParams {
  tokenId: string | undefined;
}

export async function revokeGatewayToken(
  params: RevokeGatewayTokenParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const tokenId = encodeRequired(params.tokenId, "tokenId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(`${GATEWAYS}/${id}/tokens/${tokenId}`, { token });
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function listIdentityProviders(
  _params: ListIdentityProvidersPathParams,
  getToken?: () => Promise<string>,
): Promise<IdentityProviderListResponse> {
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${GATEWAY_BASE}/identity-providers`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function discoverOidc(
  _params: DiscoverOidcPathParams,
  query: DiscoverOidcQuery,
  getToken?: () => Promise<string>,
): Promise<OidcDiscoveryResponse> {
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${GATEWAY_BASE}/identity-providers/discover`, {
    token,
    searchParams: { url: query.url },
  });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function upsertIdentityProvider(
  params: UpsertIdentityProviderPathParams,
  body: UpsertIdentityProviderRequest,
  getToken?: () => Promise<string>,
): Promise<IdentityProvider> {
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const name = encodeRequired(params.name, "name");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPUT(`${GATEWAYS}/${id}/identity-providers/${name}`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteIdentityProvider(
  params: DeleteIdentityProviderPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const id = encodeRequired(params.gatewayId, "gatewayId");
  const name = encodeRequired(params.name, "name");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(`${GATEWAYS}/${id}/identity-providers/${name}`, { token });
  if (!res.ok && res.status !== 204) throw await res.json();
}

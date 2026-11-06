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

import type { ListQuery, OrgPathParams, PaginationMeta } from "./common";

export type GatewayType = "INGRESS" | "EGRESS" | "BOTH";

export type GatewayHosting = "MANAGED" | "SELF_HOSTED";

/** A gateway without a hosting value predates self-hosted gateways: managed. */
export const isSelfHostedGateway = (gateway: {
  hosting?: GatewayHosting;
}): boolean => gateway.hosting === "SELF_HOSTED";

export const isManagedGateway = (gateway: { hosting?: GatewayHosting }): boolean =>
  !isSelfHostedGateway(gateway);

/**
 * The egress gateway the server picks in an environment when the request names none:
 * the only managed candidate. A self-hosted gateway is never picked, even a lone one.
 */
export const inferredEgressGateway = <G extends { hosting?: GatewayHosting }>(
  candidates: G[],
): G | undefined => {
  const managed = candidates.filter(isManagedGateway);
  return managed.length === 1 ? managed[0] : undefined;
};

export const gatewayHostingLabel = (gateway: { hosting?: GatewayHosting }): string =>
  isSelfHostedGateway(gateway) ? "Self-hosted" : "Managed";

/**
 * Whether to show which gateways are managed and which self-hosted. The flag
 * only gates adding self-hosted gateways: one that already exists is always
 * shown for what it is.
 */
export const showGatewayHosting = (
  flagEnabled: boolean,
  gateways: ReadonlyArray<{ hosting?: GatewayHosting }>,
): boolean => flagEnabled || gateways.some(isSelfHostedGateway);

export type GatewayStatus =
  | "ACTIVE"
  | "INACTIVE"
  | "PROVISIONING"
  | "ERROR";

export interface GatewayListenerSpec {
  port?: number;
  host?: string;
}

export interface GatewayEndpointSpec {
  http?: GatewayListenerSpec;
  https?: GatewayListenerSpec;
  tls?: GatewayListenerSpec;
}

export interface GatewayNetworkSpec {
  external?: GatewayEndpointSpec;
  internal?: GatewayEndpointSpec;
}

/**
 * Gateway configuration with ingress and egress network specs, sourced from
 * the underlying OpenChoreo environment.
 */
export interface GatewaySpec {
  ingress?: GatewayNetworkSpec;
  egress?: GatewayNetworkSpec;
}

export interface GatewayEnvironmentResponse {
  id: string;
  organizationName: string;
  name: string;
  displayName: string;
  description?: string;
  dataplaneRef: string;
  dnsPrefix: string;
  isProduction: boolean;
  gateway?: GatewaySpec;
  createdAt: string;
  updatedAt: string;
}

export interface GatewayResponse {
  uuid: string;
  organizationName: string;
  name: string;
  displayName: string;
  gatewayType: GatewayType;
  vhost: string;
  region?: string;
  isCritical: boolean;
  status: GatewayStatus;
  hosting: GatewayHosting;
  createdAt: string;
  updatedAt: string;
  environments?: GatewayEnvironmentResponse[];
}

export interface GatewayListResponse extends PaginationMeta {
  gateways: GatewayResponse[];
}

export interface CreateGatewayRequest {
  name: string;
  displayName: string;
  gatewayType: GatewayType;
  vhost: string;
  region?: string;
  isCritical?: boolean;
  environmentIds?: string[];
}

/**
 * Request body for registering a gateway the customer runs.
 * `environment` is an environment name.
 */
export interface SelfHostedGatewayRequest {
  displayName: string;
  /**
   * The gateway's name: unique in the organization and fixed once created.
   * 3-64 lowercase letters, numbers and hyphens, not starting or ending with a hyphen.
   */
  handle: string;
  description?: string;
  environment: string;
  /** At least one http(s) endpoint URL; the first is the gateway's address. */
  endpoints: string[];
  functionalityType?: "ai";
  /** Gateway version, `major.minor` or CalVer `YYYY.MM.DD`. */
  version?: string;
  isCritical?: boolean;
  properties?: Record<string, unknown>;
}

/** Handle, environment and kind are fixed at registration. */
export interface SelfHostedGatewayUpdateRequest {
  displayName?: string;
  description?: string;
  isCritical?: boolean;
  properties?: Record<string, unknown>;
}

export type CreateSelfHostedGatewayPathParams = OrgPathParams;
export type UpdateSelfHostedGatewayPathParams = GatewayPathParams;
export type DeleteSelfHostedGatewayPathParams = GatewayPathParams;
export type SelfHostedGatewayTokensPathParams = GatewayPathParams;

export interface RevokeSelfHostedGatewayTokenPathParams extends GatewayPathParams {
  tokenId: string | undefined;
}

export interface UpdateGatewayRequest {
  displayName?: string;
  isCritical?: boolean;
  status?: GatewayStatus;
}

export interface ListGatewaysQuery extends ListQuery {
  type?: GatewayType;
  status?: GatewayStatus;
  environment?: string;
}

export type ListGatewaysPathParams = OrgPathParams;
export type CreateGatewayPathParams = OrgPathParams;

export interface GatewayPathParams extends OrgPathParams {
  gatewayId: string | undefined;
}

export type GetGatewayPathParams = GatewayPathParams;
export type UpdateGatewayPathParams = GatewayPathParams;
export type DeleteGatewayPathParams = GatewayPathParams;

export type IdentityProviderType = "system" | "custom";

export interface IdentityProvider {
  name: string;
  issuer?: string;
  jwksUri?: string;
  skipTlsVerify?: boolean;
  description?: string;
  /** "system" providers (e.g. ThunderKeyManager) cannot be deleted. */
  type?: IdentityProviderType;
  /** Set on org-wide listing: the environment this provider's gateway is mapped to. */
  environmentName?: string;
  /** Set on org-wide listing: the gateway this provider is registered to. */
  gatewayId?: string;
  gatewayName?: string;
}

export interface IdentityProviderListResponse {
  count: number;
  list: IdentityProvider[];
}

export type ListIdentityProvidersPathParams = OrgPathParams;

export interface UpsertIdentityProviderPathParams extends OrgPathParams {
  gatewayId: string;
  /** Identity provider name (taken from the path; not part of the request body). */
  name: string;
}

/** Request body for creating or updating a gateway identity provider. */
export interface UpsertIdentityProviderRequest {
  issuer: string;
  jwksUri: string;
  skipTlsVerify?: boolean;
  description?: string;
}

export interface DeleteIdentityProviderPathParams extends OrgPathParams {
  gatewayId: string;
  name: string;
}

/** Issuer metadata resolved from an OpenID Connect discovery document. */
export interface OidcDiscoveryResponse {
  issuer: string;
  jwksUri: string;
}

export type DiscoverOidcPathParams = OrgPathParams;

export interface DiscoverOidcQuery {
  /** Issuer base URL or full .well-known/openid-configuration URL. */
  url: string;
}

export interface ListEnvironmentIdentityProvidersPathParams extends OrgPathParams {
  environmentId: string | undefined;
}

export interface GatewayTokenInfo {
  id: string;
  status: "active" | "revoked";
  createdAt: string;
  revokedAt?: string | null;
}

export interface GatewayTokenListResponse {
  count: number;
  list: GatewayTokenInfo[];
}

export interface GatewayTokenResponse {
  gatewayId: string;
  token: string;
  tokenId: string;
  createdAt: string;
  expiresAt?: string;
}


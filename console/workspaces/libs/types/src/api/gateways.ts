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

export type GatewayType = "AI" | "REGULAR";

export type GatewayStatus =
  | "ACTIVE"
  | "INACTIVE"
  | "PROVISIONING"
  | "ERROR";

export interface GatewayEnvironmentResponse {
  id: string;
  organizationName: string;
  name: string;
  displayName: string;
  description?: string;
  dataplaneRef: string;
  dnsPrefix: string;
  isProduction: boolean;
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


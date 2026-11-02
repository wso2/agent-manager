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

import { useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";
import type {
  DeleteGatewayPathParams,
  DiscoverOidcPathParams,
  DiscoverOidcQuery,
  GatewayListResponse,
  OidcDiscoveryResponse,
  GatewayResponse,
  GetGatewayPathParams,
  IdentityProviderListResponse,
  ListEnvironmentIdentityProvidersPathParams,
  ListGatewaysPathParams,
  ListGatewaysQuery,
  ListIdentityProvidersPathParams,
  UpsertIdentityProviderPathParams,
  UpsertIdentityProviderRequest,
  DeleteIdentityProviderPathParams,
  IdentityProvider,
  UpdateGatewayPathParams,
  UpdateGatewayRequest,
  CreateSelfHostedGatewayPathParams,
  DeleteSelfHostedGatewayPathParams,
  GatewayTokenListResponse,
  GatewayTokenResponse,
  ListSelfHostedGatewaysPathParams,
  ListSelfHostedGatewaysQuery,
  RevokeSelfHostedGatewayTokenPathParams,
  SelfHostedGatewayRequest,
  SelfHostedGatewayTokensPathParams,
  SelfHostedGatewayUpdateRequest,
  UpdateSelfHostedGatewayPathParams,
} from "@agent-management-platform/types";
import {
  assignGatewayToEnvironment,
  createSelfHostedGateway,
  createSelfHostedGatewayToken,
  deleteSelfHostedGateway,
  listSelfHostedGatewayTokens,
  listSelfHostedGateways,
  revokeSelfHostedGatewayToken,
  updateSelfHostedGateway,
  deleteGateway,
  deleteIdentityProvider,
  discoverOidc,
  getGateway,
  listEnvironmentIdentityProviders,
  listGatewayTokens,
  listGateways,
  listIdentityProviders,
  removeGatewayFromEnvironment,
  revokeGatewayToken,
  rotateGatewayToken,
  updateGateway,
  upsertIdentityProvider,
} from "../apis";

export function useListGateways(
  params: ListGatewaysPathParams,
  query?: ListGatewaysQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<GatewayListResponse>({
    queryKey: ["gateways", params, query],
    queryFn: () => listGateways(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetGateway(params: GetGatewayPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<GatewayResponse>({
    queryKey: ["gateway", params],
    queryFn: () => getGateway(params, getToken),
    enabled: !!params.orgName && !!params.gatewayId,
  });
}

export function useListIdentityProviders(
  params: ListIdentityProvidersPathParams,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<IdentityProviderListResponse>({
    queryKey: ["identity-providers", params],
    queryFn: () => listIdentityProviders(params, getToken),
    enabled: !!params.orgName,
  });
}

/**
 * Resolves issuer + JWKS URI from an OIDC discovery URL to auto-fill the Add
 * Identity Provider dialog. Triggered on demand (button click); notifications
 * are suppressed so the dialog can surface success/failure inline.
 */
export function useDiscoverOidc(params: DiscoverOidcPathParams) {
  const { getToken } = useAuthHooks();
  return useApiMutation<OidcDiscoveryResponse, unknown, DiscoverOidcQuery>({
    mutationFn: (query) => discoverOidc(params, query, getToken),
    showSuccess: false,
    showError: false,
  });
}

export function useListEnvironmentIdentityProviders(
  params: ListEnvironmentIdentityProvidersPathParams,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<IdentityProviderListResponse>({
    queryKey: ["environment-identity-providers", params],
    queryFn: () => listEnvironmentIdentityProviders(params, getToken),
    enabled: !!params.orgName && !!params.environmentId,
  });
}

/**
 * Upserts a gateway identity provider via the REST API. Used by the managed
 * (cloud) path of the Add Identity Provider dialog, where the server also patches
 * the gateway runtime config; the self-hosted path renders the script instead.
 */
export function useUpsertIdentityProvider() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    IdentityProvider,
    unknown,
    { params: UpsertIdentityProviderPathParams; body: UpsertIdentityProviderRequest }
  >({
    action: { verb: 'create', target: 'identity provider' },
    mutationFn: ({ params, body }) => upsertIdentityProvider(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["identity-providers"] });
      queryClient.invalidateQueries({ queryKey: ["environment-identity-providers"] });
    },
  });
}

export function useDeleteIdentityProvider() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteIdentityProviderPathParams>({
    action: { verb: 'remove', target: 'identity provider' },
    mutationFn: (params) => deleteIdentityProvider(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["identity-providers"] });
      queryClient.invalidateQueries({ queryKey: ["environment-identity-providers"] });
    },
  });
}

export function useUpdateGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    GatewayResponse,
    unknown,
    { params: UpdateGatewayPathParams; body: UpdateGatewayRequest }
  >({
    action: { verb: 'update', target: 'gateway' },
    mutationFn: ({ params, body }) => updateGateway(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateways"] });
      queryClient.invalidateQueries({ queryKey: ["gateway"] });
    },
  });
}

export function useDeleteGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteGatewayPathParams>({
    action: { verb: 'delete', target: 'gateway' },
    mutationFn: (params) => deleteGateway(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateways"] });
    },
  });
}

export function useAssignGatewayToEnvironment() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    GatewayResponse,
    unknown,
    import("../apis").AssignGatewayToEnvironmentParams
  >({
    action: { verb: 'assign', target: 'gateway to environment' },
    mutationFn: (params) => assignGatewayToEnvironment(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateways"] });
      queryClient.invalidateQueries({ queryKey: ["gateway"] });
    },
  });
}

export function useListGatewayTokens(
  params: import("../apis").ListGatewayTokensParams,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery({
    queryKey: ["gateway-tokens", params],
    queryFn: () => listGatewayTokens(params, getToken),
    enabled: !!params.orgName && !!params.gatewayId,
  });
}

export function useRotateGatewayToken() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation({
    action: { verb: 'rotate', target: 'gateway token' },
    mutationFn: (
      params: import("../apis").ListGatewayTokensParams,
    ) => rotateGatewayToken(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateway-tokens"] });
    },
  });
}

export function useRevokeGatewayToken() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation({
    action: { verb: 'revoke', target: 'gateway token' },
    mutationFn: (params: {
      orgName: string;
      gatewayId: string;
      tokenId: string;
    }) => revokeGatewayToken(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateway-tokens"] });
    },
  });
}

export function useRemoveGatewayFromEnvironment() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    void,
    unknown,
    import("../apis").RemoveGatewayFromEnvironmentParams
  >({
    action: { verb: 'remove', target: 'gateway from environment' },
    mutationFn: (params) => removeGatewayFromEnvironment(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateways"] });
      queryClient.invalidateQueries({ queryKey: ["gateway"] });
    },
  });
}


function invalidateGatewayQueries(queryClient: ReturnType<typeof useQueryClient>) {
  queryClient.invalidateQueries({ queryKey: ["gateways"] });
  queryClient.invalidateQueries({ queryKey: ["self-hosted-gateways"] });
}

export function useListSelfHostedGateways(
  params: ListSelfHostedGatewaysPathParams,
  query?: ListSelfHostedGatewaysQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<GatewayListResponse>({
    queryKey: ["self-hosted-gateways", params, query],
    queryFn: () => listSelfHostedGateways(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useCreateSelfHostedGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    GatewayResponse,
    unknown,
    { params: CreateSelfHostedGatewayPathParams; body: SelfHostedGatewayRequest }
  >({
    action: { verb: 'create', target: 'self-hosted gateway' },
    mutationFn: ({ params, body }) => createSelfHostedGateway(params, body, getToken),
    onSuccess: () => invalidateGatewayQueries(queryClient),
  });
}

export function useUpdateSelfHostedGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    GatewayResponse,
    unknown,
    { params: UpdateSelfHostedGatewayPathParams; body: SelfHostedGatewayUpdateRequest }
  >({
    action: { verb: 'update', target: 'self-hosted gateway' },
    mutationFn: ({ params, body }) => updateSelfHostedGateway(params, body, getToken),
    onSuccess: () => invalidateGatewayQueries(queryClient),
  });
}

export function useDeleteSelfHostedGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteSelfHostedGatewayPathParams>({
    action: { verb: 'delete', target: 'self-hosted gateway' },
    mutationFn: (params) => deleteSelfHostedGateway(params, getToken),
    onSuccess: () => invalidateGatewayQueries(queryClient),
  });
}

export function useSelfHostedGatewayTokens(params: SelfHostedGatewayTokensPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<GatewayTokenListResponse>({
    queryKey: ["self-hosted-gateways", "tokens", params],
    queryFn: () => listSelfHostedGatewayTokens(params, getToken),
    enabled: !!params.orgName && !!params.gatewayId,
  });
}

export function useCreateSelfHostedGatewayToken() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<GatewayTokenResponse, unknown, SelfHostedGatewayTokensPathParams>({
    action: { verb: 'create', target: 'registration token' },
    mutationFn: (params) => createSelfHostedGatewayToken(params, getToken),
    onSuccess: () => invalidateGatewayQueries(queryClient),
  });
}

export function useRevokeSelfHostedGatewayToken() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, RevokeSelfHostedGatewayTokenPathParams>({
    action: { verb: 'revoke', target: 'registration token' },
    mutationFn: (params) => revokeSelfHostedGatewayToken(params, getToken),
    onSuccess: () => invalidateGatewayQueries(queryClient),
  });
}

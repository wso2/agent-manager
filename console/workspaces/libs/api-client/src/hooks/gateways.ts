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

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  type CreateGatewayPathParams,
  type CreateGatewayRequest,
  type DeleteGatewayPathParams,
  type GatewayListResponse,
  type GatewayResponse,
  type GetGatewayPathParams,
  type ListGatewaysPathParams,
  type ListGatewaysQuery,
  type UpdateGatewayPathParams,
  type UpdateGatewayRequest,
} from "@agent-management-platform/types";
import {
  assignGatewayToEnvironment,
  createGateway,
  deleteGateway,
  getGateway,
  listGatewayTokens,
  listGateways,
  removeGatewayFromEnvironment,
  revokeGatewayToken,
  rotateGatewayToken,
  updateGateway,
} from "../apis";

export function useListGateways(
  params: ListGatewaysPathParams,
  query?: ListGatewaysQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<GatewayListResponse>({
    queryKey: ["gateways", params, query],
    queryFn: () => listGateways(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetGateway(params: GetGatewayPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<GatewayResponse>({
    queryKey: ["gateway", params],
    queryFn: () => getGateway(params, getToken),
    enabled: !!params.orgName && !!params.gatewayId,
  });
}

export function useCreateGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    GatewayResponse,
    unknown,
    { params: CreateGatewayPathParams; body: CreateGatewayRequest }
  >({
    mutationFn: ({ params, body }) => createGateway(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateways"] });
    },
  });
}

export function useUpdateGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    GatewayResponse,
    unknown,
    { params: UpdateGatewayPathParams; body: UpdateGatewayRequest }
  >({
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
  return useMutation<void, unknown, DeleteGatewayPathParams>({
    mutationFn: (params) => deleteGateway(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateways"] });
    },
  });
}

export function useAssignGatewayToEnvironment() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    GatewayResponse,
    unknown,
    import("../apis").AssignGatewayToEnvironmentParams
  >({
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
  return useQuery({
    queryKey: ["gateway-tokens", params],
    queryFn: () => listGatewayTokens(params, getToken),
    enabled: !!params.orgName && !!params.gatewayId,
  });
}

export function useRotateGatewayToken() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation({
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
  return useMutation({
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
  return useMutation<
    void,
    unknown,
    import("../apis").RemoveGatewayFromEnvironmentParams
  >({
    mutationFn: (params) => removeGatewayFromEnvironment(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["gateways"] });
      queryClient.invalidateQueries({ queryKey: ["gateway"] });
    },
  });
}


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

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createOrganization, generateResourceName, getOrganization, listOrganizations } from "../apis";
import {
  OrganizationListResponse,
  OrganizationResponse,
  CreateOrganizationRequest,
  GetOrganizationPathParams,
  ListOrganizationsQuery,
  ResourceNameRequest,
  ResourceNameResponse,
  GenerateResourceNamePathParams,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";

export function useListOrganizations(
  query?: ListOrganizationsQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<OrganizationListResponse>({
    queryKey: ['organizations', query],
    queryFn: () => listOrganizations(query, getToken),
    retry: false,
  });
}

export function useGetOrganization(params: GetOrganizationPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<OrganizationResponse>({
    queryKey: ['organization', params],
    queryFn: () => getOrganization(params, getToken),
    enabled: !!params.orgName,
  });
}

export function useCreateOrganization() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    OrganizationResponse,
    unknown,
    CreateOrganizationRequest
  >({
    mutationFn: (body) => createOrganization(body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['organizations'] });
    },
  });
}

export function useGenerateResourceName(params: GenerateResourceNamePathParams) {
  const { getToken } = useAuthHooks();
  return useMutation<
    ResourceNameResponse,
    unknown,
    ResourceNameRequest
  >({
    mutationFn: (body) => generateResourceName(params, body, getToken),
  });
}


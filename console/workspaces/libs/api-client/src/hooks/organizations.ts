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
  });
}

export function useGetOrganization(params: GetOrganizationPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<OrganizationResponse>({
    queryKey: ['organization', params],
    queryFn: () => getOrganization(params, getToken),
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


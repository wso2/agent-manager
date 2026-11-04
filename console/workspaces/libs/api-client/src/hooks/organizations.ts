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

import { useQueryClient } from "@tanstack/react-query";
import { generatePath } from "react-router-dom";
import { createOrganization, generateResourceName, getOrganization, listOrganizations } from "../apis";
import {
  absoluteRouteMap,
  type OrganizationListItem,
  type OrganizationListResponse,
  type OrganizationResponse,
  type CreateOrganizationRequest,
  type GetOrganizationPathParams,
  type ListOrganizationsQuery,
  type ResourceNameRequest,
  type ResourceNameResponse,
  type GenerateResourceNamePathParams,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

/**
 * The `:orgId` the console puts in its URLs for an org: the org handle when the
 * backend reports one, else the name (the OpenChoreo namespace), as before.
 */
type OrgRouteSource = Pick<OrganizationListItem, 'name' | 'handle'>;

export function orgRouteId(org: OrgRouteSource): string;
export function orgRouteId(org: OrgRouteSource | undefined): string | undefined;
export function orgRouteId(org: OrgRouteSource | undefined): string | undefined {
  return org?.handle || org?.name;
}

/**
 * Where to send a link that names the org the old way — by its name (the
 * namespace, e.g. `/org/wc-01a0c318-7a4ef447/...`) — so it keeps working now
 * that URLs carry the handle. Undefined when the link needs no redirect.
 */
export function legacyOrgRedirectPath(
  pathname: string,
  orgId: string | undefined,
  orgs: Pick<OrganizationListItem, 'name' | 'handle'>[] | undefined,
): string | undefined {
  if (!orgId || orgs?.some((o) => orgRouteId(o) === orgId)) {
    return undefined;
  }
  const routeId = orgRouteId(orgs?.find((o) => o.name === orgId));
  const oldBase = generatePath(absoluteRouteMap.children.org.path, { orgId });
  if (!routeId || (pathname !== oldBase && !pathname.startsWith(`${oldBase}/`))) {
    return undefined;
  }
  return generatePath(absoluteRouteMap.children.org.path, { orgId: routeId })
    + pathname.slice(oldBase.length);
}

export function useListOrganizations(
  query?: ListOrganizationsQuery,
) {
  const { getToken, isAuthenticated, userInfo } = useAuthHooks();
  return useApiQuery<OrganizationListResponse>({
    queryKey: ['organizations', query, isAuthenticated, userInfo?.sub],
    queryFn: () => listOrganizations(query, getToken),
    retry: false,
    retryOnMount: false,
  });
}

export function useGetOrganization(params: GetOrganizationPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<OrganizationResponse>({
    queryKey: ['organization', params],
    queryFn: () => getOrganization(params, getToken),
    enabled: !!params.orgName,
  });
}

export function useCreateOrganization() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    OrganizationResponse,
    unknown,
    CreateOrganizationRequest
  >({
    action: { verb: 'create', target: 'organization' },
    mutationFn: (body) => createOrganization(body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['organizations'] });
    },
  });
}

export function useGenerateResourceName(params: GenerateResourceNamePathParams) {
  const { getToken } = useAuthHooks();
  return useApiMutation<
    ResourceNameResponse,
    unknown,
    ResourceNameRequest
  >({
    action: { verb: 'generate', target: 'resource name' },
    showSuccess: false,
    mutationFn: (body) => generateResourceName(params, body, getToken),
    mutationKey: ['generate-resource-name', params],
  });
}


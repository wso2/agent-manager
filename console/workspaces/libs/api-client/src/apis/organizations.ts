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

import { httpGET, httpPOST, SERVICE_BASE } from "../utils";
import {
  OrganizationListResponse,
  OrganizationResponse,
  CreateOrganizationRequest,
  ListOrganizationsQuery,
  GetOrganizationPathParams,
  ResourceNameRequest,
  ResourceNameResponse,
  GenerateResourceNamePathParams,
} from "@agent-management-platform/types";

export async function listOrganizations(
  query?: ListOrganizationsQuery,
  getToken?: () => Promise<string>
): Promise<OrganizationListResponse> {
  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          .filter(([_, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${SERVICE_BASE}/orgs`, {
    searchParams: search,
    token,
  });

  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createOrganization(
  body: CreateOrganizationRequest,
  getToken?: () => Promise<string>
): Promise<OrganizationResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${SERVICE_BASE}/orgs`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getOrganization(
  params: GetOrganizationPathParams,
  getToken?: () => Promise<string>
): Promise<OrganizationResponse> {
  const { orgName } = params;
  
  if (!orgName) {
    throw new Error("orgName is required");
  }
  
  const token = getToken ? await getToken() : undefined;
  const url = `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}`;
  const res = await httpGET(url, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function generateResourceName(
  params: GenerateResourceNamePathParams,
  body: ResourceNameRequest,
  getToken?: () => Promise<string>
): Promise<ResourceNameResponse> {
  const { orgName } = params;
  
  if (!orgName) {
    throw new Error("orgName is required");
  }
  
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/utils/generate-name`,
    body,
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

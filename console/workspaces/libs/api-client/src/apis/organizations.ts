import { httpGET, httpPOST, SERVICE_BASE } from '../utils';
import {
  OrganizationListResponse,
  OrganizationResponse,
  CreateOrganizationRequest,
  ListOrganizationsQuery,
  GetOrganizationPathParams,
  ResourceNameRequest,
  ResourceNameResponse,
  GenerateResourceNamePathParams,
} from '@agent-management-platform/types';

export async function listOrganizations(
  query?: ListOrganizationsQuery,
  getToken?: () => Promise<string>,
): Promise<OrganizationListResponse> {
  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          .filter(([_, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)]),
      )
    : undefined;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs`,
    { searchParams: search, token },
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createOrganization(
  body: CreateOrganizationRequest,
  getToken?: () => Promise<string>,
): Promise<OrganizationResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getOrganization(
  params: GetOrganizationPathParams,
  getToken?: () => Promise<string>,
): Promise<OrganizationResponse> {
  const { orgName } = params;
  const token = getToken ? await getToken() : undefined;
  const url = `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}`;
  const res = await httpGET(url, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function generateResourceName(
  params: GenerateResourceNamePathParams,
  body: ResourceNameRequest,
  getToken?: () => Promise<string>,
): Promise<ResourceNameResponse> {
  const { orgName } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/generate-name`,
    body,
    { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}


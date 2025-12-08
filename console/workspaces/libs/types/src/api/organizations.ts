import { type ListQuery, type PaginationMeta } from './common';

// Requests
export interface CreateOrganizationRequest {
  name: string;
}

// Responses
export interface OrganizationResponse {
  name: string;
  displayName: string;
  description: string;
  namespace: string;
  createdAt: string; // ISO date-time
}

export interface OrganizationListResponse extends PaginationMeta {
  organizations: OrganizationResponse[];
}

// Path/Query helpers
export type ListOrganizationsQuery = ListQuery;
export type GetOrganizationPathParams = { orgName: string };


import { type ListQuery, type PaginationMeta } from './common';

// Requests
export interface CreateProjectRequest {
  name: string;
  displayName: string;
  description?: string;
  deploymentPipeline: string;
}

// Responses
export interface ProjectResponse {
  name: string;
  orgName: string;
  displayName: string;
  description: string;
  deploymentPipeline: string;
  createdAt: string; // ISO date-time
}

export interface ProjectListResponse extends PaginationMeta {
  projects: ProjectResponse[];
}

// Path/Query helpers
export type ListProjectsPathParams = { orgName: string };
export type CreateProjectPathParams = { orgName: string };
export type GetProjectPathParams = { orgName: string; projName: string };
export type ListProjectsQuery = ListQuery;


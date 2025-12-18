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
export type ListProjectsPathParams = { orgName: string | undefined };
export type CreateProjectPathParams = { orgName: string | undefined };
export type GetProjectPathParams = { orgName: string | undefined; projName: string | undefined };
export type ListProjectsQuery = ListQuery;
export type DeleteProjectPathParams = { orgName: string | undefined; projName: string | undefined };


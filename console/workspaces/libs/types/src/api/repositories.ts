/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

// Request types
export interface ListBranchesRequest {
  owner: string;
  repository: string;
}

export interface ListCommitsRequest {
  owner: string;
  repo: string;
  branch?: string;
  path?: string;
  author?: string;
  since?: string;
  until?: string;
}

// Query parameters
export interface ListBranchesQuery {
  limit?: number;
  offset?: number;
}

export interface ListCommitsQuery {
  limit?: number;
  offset?: number;
}

// Response types
export interface Branch {
  name: string;
  commitSha: string;
  isDefault: boolean;
}

export interface CommitAuthor {
  name: string;
  email: string;
  avatarUrl?: string;
}

export interface Commit {
  sha: string;
  shortSha: string;
  message: string;
  author: CommitAuthor;
  timestamp: string;
  isLatest: boolean;
}

export interface ListBranchesResponse {
  branches: Branch[];
  nextOffset?: number;
  limit: number;
  offset: number;
}

export interface ListCommitsResponse {
  commits: Commit[];
  nextOffset?: number;
  limit: number;
  offset: number;
}

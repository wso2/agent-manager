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

import { listBranches, listCommits } from "../apis";
import type {
  ListBranchesRequest,
  ListBranchesResponse,
  ListBranchesQuery,
  ListCommitsRequest,
  ListCommitsResponse,
  ListCommitsQuery,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiQuery } from "./react-query-notifications";

export function useListBranches(
  orgName: string,
  body: ListBranchesRequest,
  query?: ListBranchesQuery,
  enabled: boolean = true,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ListBranchesResponse>({
    queryKey: [
      "branches",
      {
        orgName,
        owner: body.owner,
        repository: body.repository,
        secretRef: body.secretRef,
      },
      query,
    ],
    queryFn: () => listBranches(orgName, body, query, getToken),
    enabled: enabled && !!orgName && !!body.owner && !!body.repository,
  });
}

export function useListCommits(
  orgName: string,
  body: ListCommitsRequest,
  query?: ListCommitsQuery,
  enabled: boolean = true,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ListCommitsResponse>({
    queryKey: [
      "commits",
      {
        orgName,
        owner: body.owner,
        repo: body.repo,
        branch: body.branch,
        secretRef: body.secretRef,
        projectName: body.projectName,
        componentName: body.componentName,
      },
      query,
    ],
    queryFn: () => listCommits(orgName, body, query, getToken),
    enabled: enabled && !!orgName && !!body.owner && !!body.repo,
  });
}

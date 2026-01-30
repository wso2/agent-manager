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

import { SERVICE_BASE } from "../utils";
import {
  ListBranchesRequest,
  ListBranchesResponse,
  ListBranchesQuery,
  ListCommitsRequest,
  ListCommitsResponse,
  ListCommitsQuery,
  globalConfig,
} from "@agent-management-platform/types";

export async function listBranches(
  body: ListBranchesRequest,
  query?: ListBranchesQuery,
  getToken?: () => Promise<string>,
): Promise<ListBranchesResponse> {
  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          .filter(([, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;

  const token = getToken ? await getToken() : undefined;
  const baseUrl = globalConfig.apiBaseUrl;

  const requestHeaders: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    requestHeaders["Authorization"] = `Bearer ${token}`;
  }

  const response = await fetch(
    `${baseUrl}${SERVICE_BASE}/repositories/branches?${new URLSearchParams(search).toString()}`,
    {
      method: "POST",
      headers: requestHeaders,
      body: JSON.stringify(body),
    }
  );

  if (!response.ok) {
    let errorBody;
    try {
      errorBody = await response.json();
    } catch {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
    throw errorBody;
  }
  return response.json();
}

export async function listCommits(
  body: ListCommitsRequest,
  query?: ListCommitsQuery,
  getToken?: () => Promise<string>,
): Promise<ListCommitsResponse> {
  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          .filter(([, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;

  const token = getToken ? await getToken() : undefined;
  const baseUrl = globalConfig.apiBaseUrl;

  const requestHeaders: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    requestHeaders["Authorization"] = `Bearer ${token}`;
  }

  const response = await fetch(
    `${baseUrl}${SERVICE_BASE}/repositories/commits?${new URLSearchParams(search).toString()}`,
    {
      method: "POST",
      headers: requestHeaders,
      body: JSON.stringify(body),
    }
  );

  if (!response.ok) {
    let errorBody;
    try {
      errorBody = await response.json();
    } catch {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
    throw errorBody;
  }
  return response.json();
}

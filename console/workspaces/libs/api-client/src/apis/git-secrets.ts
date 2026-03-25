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

import { httpDELETE, httpGET, httpPOST, SERVICE_BASE } from "../utils";
import {
  GitSecretListResponse,
  GitSecretResponse,
  CreateGitSecretRequest,
  ListGitSecretsPathParams,
  CreateGitSecretPathParams,
  GetGitSecretPathParams,
  DeleteGitSecretPathParams,
  ListGitSecretsQuery,
} from "@agent-management-platform/types";

/**
 * List all git secrets for an organization
 */
export async function listGitSecrets(
  params: ListGitSecretsPathParams,
  query?: ListGitSecretsQuery,
  getToken?: () => Promise<string>,
): Promise<GitSecretListResponse> {
  const { orgName = "default" } = params;

  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          .filter(([, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;

  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/git-secrets`,
    { searchParams: search, token }
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Create a new git secret
 */
export async function createGitSecret(
  params: CreateGitSecretPathParams,
  body: CreateGitSecretRequest,
  getToken?: () => Promise<string>,
): Promise<GitSecretResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/git-secrets`,
    body,
    { token }
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Get a git secret by name
 */
export async function getGitSecret(
  params: GetGitSecretPathParams,
  getToken?: () => Promise<string>,
): Promise<GitSecretResponse> {
  const { orgName = "default", secretName } = params;

  if (!secretName) {
    throw new Error("secretName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/git-secrets/${encodeURIComponent(secretName)}`,
    { token }
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Delete a git secret
 */
export async function deleteGitSecret(
  params: DeleteGitSecretPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", secretName } = params;

  if (!secretName) {
    throw new Error("secretName is required");
  }

  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/git-secrets/${encodeURIComponent(secretName)}`,
    { token }
  );

  if (!res.ok) throw await res.json();
}

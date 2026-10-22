/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import type {
  A2ACatalogAgent,
  A2ACatalogEntryPathParams,
  A2ACatalogListPathParams,
  A2ACatalogListQuery,
  A2ACatalogListResponse,
  A2AConnectPathParams,
  A2AConnectResponse,
} from "@agent-management-platform/types";
import { encodeRequired, httpGET, SERVICE_BASE } from "../utils";
import { agentEnvironmentResourcePath } from "./agent-card";

function catalogSearchParams(query: A2ACatalogListQuery): Record<string, string> | undefined {
  const present = Object.entries(query).filter(([, value]) => value !== undefined && value !== "");
  if (!present.length) return undefined;
  return Object.fromEntries(present.map(([key, value]) => [key, String(value)]));
}

export async function listA2ACatalog(
  params: A2ACatalogListPathParams,
  query: A2ACatalogListQuery,
  getToken?: () => Promise<string>,
): Promise<A2ACatalogListResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeRequired(params.orgName, "orgName")}/a2a-agents`,
    { searchParams: catalogSearchParams(query), token },
  );
  return res.json();
}

// An agent outside the catalogue answers 404, which the http helpers throw with status 404.
export async function getA2ACatalogEntry(
  params: A2ACatalogEntryPathParams,
  getToken?: () => Promise<string>,
): Promise<A2ACatalogAgent> {
  const token = getToken ? await getToken() : undefined;
  const org = encodeRequired(params.orgName, "orgName");
  const project = encodeRequired(params.projName, "projName");
  const agent = encodeRequired(params.agentName, "agentName");
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/projects/${project}/agents/${agent}/a2a-catalog-entry`,
    { token },
  );
  return res.json();
}

export async function getA2AConnect(
  params: A2AConnectPathParams,
  getToken?: () => Promise<string>,
): Promise<A2AConnectResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(agentEnvironmentResourcePath(params, "a2a-connect"), { token });
  return res.json();
}

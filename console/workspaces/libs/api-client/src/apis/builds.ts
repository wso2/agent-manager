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
  BuildAgentPathParams,
  BuildAgentQuery,
  BuildResponse,
  BuildsListResponse,
  GetAgentBuildsPathParams,
  GetAgentBuildsQuery,
  GetBuildLogsPathParams,
  GetBuildPathParams,
  BuildDetailsResponse,
  BuildLogEntry,
} from "@agent-management-platform/types/src/api/builds";

export async function buildAgent(
  params: BuildAgentPathParams,
  query?: BuildAgentQuery,
  getToken?: () => Promise<string>
): Promise<BuildResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  
  if (!agentName) {
    throw new Error("agentName is required");
  }
  
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(
      orgName
    )}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(
      agentName
    )}/builds`,
    {},
    {
      searchParams: query?.commitId ? { commitId: query.commitId } : undefined,
      token,
    }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getAgentBuilds(
  params: GetAgentBuildsPathParams,
  query?: GetAgentBuildsQuery,
  getToken?: () => Promise<string>
): Promise<BuildsListResponse> {
  const { orgName = "default", projName = "default", agentName } = params;
  
  if (!agentName) {
    throw new Error("agentName is required");
  }
  
  const search = query
    ? Object.fromEntries(
        Object.entries(query)
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          .filter(([_, v]) => v !== undefined)
          .map(([k, v]) => [k, String(v)])
      )
    : undefined;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(
      orgName
    )}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(
      agentName
    )}/builds`,
    { searchParams: search, token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

// eslint-disable-next-line max-len
export async function getBuild(
  params: GetBuildPathParams,
  getToken?: () => Promise<string>
): Promise<BuildDetailsResponse> {
  const {
    orgName = "default",
    projName = "default",
    agentName,
    buildName,
  } = params;
  
  if (!agentName) {
    throw new Error("agentName is required");
  }
  if (!buildName) {
    throw new Error("buildName is required");
  }
  
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(
      orgName
    )}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(
      agentName
    )}/builds/${encodeURIComponent(buildName)}`,
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getBuildLogs(
  params: GetBuildLogsPathParams,
  getToken?: () => Promise<string>
): Promise<BuildLogEntry[]> {
  const {
    orgName = "default",
    projName = "default",
    agentName,
    buildName,
  } = params;
  
  if (!agentName) {
    throw new Error("agentName is required");
  }
  if (!buildName) {
    throw new Error("buildName is required");
  }
  
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(
      orgName
    )}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(
      agentName
    )}/builds/${encodeURIComponent(buildName)}/build-logs`,
    { token }
  );
  if (!res.ok) throw await res.json();
  const resultJson = await res.json();
  return resultJson.logs ?? [];
}

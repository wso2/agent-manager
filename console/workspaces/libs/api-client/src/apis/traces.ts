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

import {
  GetTraceListPathParams,
  GetTracePathParams,
  TraceDetailsResponse,
  TraceListResponse,
} from "@agent-management-platform/types";
import { httpGET, SERVICE_BASE } from "../utils";

export async function getTrace(
  params: GetTracePathParams,
  getToken?: () => Promise<string>
): Promise<TraceDetailsResponse> {
  const { agentName, traceId, projName, orgName, envId } = params;

  const missingParams: string[] = [];
  if (!agentName) {
    missingParams.push("agentName");
  }
  if (!traceId) {
     missingParams.push("traceId");
  }
  if (!projName) {
    missingParams.push("projName");
  }
  if (!orgName) {
    missingParams.push("orgName");
  }
  if (!envId) {
    missingParams.push("envId");
  }
  
  if (missingParams.length > 0) {
    throw new Error(`Missing required parameters: ${missingParams.join(", ")}`);
  }
  const token = getToken ? await getToken() : undefined;
  
  // Note: envId is validated but not sent to backend as per Go service implementation
  // API path: GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/trace/{traceId}
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${orgName}/projects/${projName}/agents/${agentName}/trace/${traceId}`,
    {
      token,
    }
  );

  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getTraceList(
  params: GetTraceListPathParams,
  getToken?: () => Promise<string>
): Promise<TraceListResponse> {
  const {
    agentName,
    startTime,
    endTime,
    projName,
    orgName,
    envId,
    limit,
    offset,
    sortOrder,
  } = params;

  const missingParams: string[] = [];
  if (!agentName) missingParams.push("agentName");
  if (!projName) missingParams.push("projName");
  if (!orgName) missingParams.push("orgName");
  if (!envId) missingParams.push("envId");
  
  if (missingParams.length > 0) {
    throw new Error(`Missing required parameters: ${missingParams.join(", ")}`);
  }
  const token = getToken ? await getToken() : undefined;

  const searchParams: Record<string, string> = {};
  if (startTime) searchParams.startTime = startTime;
  if (endTime) searchParams.endTime = endTime;
  if (limit !== undefined) searchParams.limit = limit.toString();
  if (offset !== undefined) searchParams.offset = offset.toString();
  if (sortOrder) searchParams.sortOrder = sortOrder;

  // Note: envId is validated but not sent to backend as per Go service implementation
  // API path: GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/traces
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${orgName}/projects/${projName}/agents/${agentName}/traces`,
    {
      searchParams: Object.keys(searchParams).length > 0 ? searchParams : undefined,
      token,
    }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

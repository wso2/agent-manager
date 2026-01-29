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

import { cloneDeep } from "lodash";
import { httpPOST, SERVICE_BASE } from "../utils";
import {
  FilterAgentRuntimeLogsPathParams,
  LogFilterRequest,
  LogsResponse,
} from "@agent-management-platform/types";

export async function filterAgentRuntimeLogs(
  params: FilterAgentRuntimeLogsPathParams,
  body: LogFilterRequest,
  getToken?: () => Promise<string>,
): Promise<LogsResponse> {
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
    )}/runtime-logs`,
    cloneDeep(body),
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

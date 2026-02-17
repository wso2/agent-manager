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

import {
  type CreateMonitorPathParams,
  type CreateMonitorRequest,
  type DeleteMonitorPathParams,
  type GetMonitorPathParams,
  type ListMonitorRunsPathParams,
  type ListMonitorsPathParams,
  type MonitorListResponse,
  type MonitorResponse,
  type MonitorRunListResponse,
  type MonitorRunLogsPathParams,
  type MonitorRunResponse,
  type RerunMonitorPathParams,
  type StartMonitorPathParams,
  type StopMonitorPathParams,
  type UpdateMonitorPathParams,
  type UpdateMonitorRequest,
  type LogsResponse,
} from "@agent-management-platform/types";
import { httpDELETE, httpGET, httpPATCH, httpPOST, SERVICE_BASE } from "../utils";

export async function listMonitors(
  params: ListMonitorsPathParams,
  getToken?: () => Promise<string>
): Promise<MonitorListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/monitors`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createMonitor(
  params: CreateMonitorPathParams,
  body: CreateMonitorRequest,
  getToken?: () => Promise<string>
): Promise<MonitorResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(`${SERVICE_BASE}/orgs/${org}/monitors`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getMonitor(
  params: GetMonitorPathParams,
  getToken?: () => Promise<string>
): Promise<MonitorResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const monitor = encodeRequired(params.monitorName, "monitorName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(`${SERVICE_BASE}/orgs/${org}/monitors/${monitor}`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateMonitor(
  params: UpdateMonitorPathParams,
  body: UpdateMonitorRequest,
  getToken?: () => Promise<string>
): Promise<MonitorResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const monitor = encodeRequired(params.monitorName, "monitorName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPATCH(`${SERVICE_BASE}/orgs/${org}/monitors/${monitor}`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteMonitor(
  params: DeleteMonitorPathParams,
  getToken?: () => Promise<string>
): Promise<void> {
  const org = encodeRequired(params.orgName, "orgName");
  const monitor = encodeRequired(params.monitorName, "monitorName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpDELETE(`${SERVICE_BASE}/orgs/${org}/monitors/${monitor}`, { token });
  if (!res.ok) throw await res.json();
  if (res.status === 204 || res.headers.get("content-length") === "0") {
    return;
  }
  await res.json();
}

export async function stopMonitor(
  params: StopMonitorPathParams,
  getToken?: () => Promise<string>
): Promise<MonitorResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const monitor = encodeRequired(params.monitorName, "monitorName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/monitors/${monitor}/stop`,
    {},
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function startMonitor(
  params: StartMonitorPathParams,
  getToken?: () => Promise<string>
): Promise<MonitorResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const monitor = encodeRequired(params.monitorName, "monitorName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/monitors/${monitor}/start`,
    {},
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listMonitorRuns(
  params: ListMonitorRunsPathParams,
  getToken?: () => Promise<string>
): Promise<MonitorRunListResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const monitor = encodeRequired(params.monitorName, "monitorName");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/monitors/${monitor}/runs`,
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function rerunMonitor(
  params: RerunMonitorPathParams,
  getToken?: () => Promise<string>
): Promise<MonitorRunResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const monitor = encodeRequired(params.monitorName, "monitorName");
  const run = encodeRequired(params.runId, "runId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpPOST(
    `${SERVICE_BASE}/orgs/${org}/monitors/${monitor}/runs/${run}/rerun`,
    {},
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getMonitorRunLogs(
  params: MonitorRunLogsPathParams,
  getToken?: () => Promise<string>
): Promise<LogsResponse> {
  const org = encodeRequired(params.orgName, "orgName");
  const monitor = encodeRequired(params.monitorName, "monitorName");
  const run = encodeRequired(params.runId, "runId");
  const token = getToken ? await getToken() : undefined;

  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${org}/monitors/${monitor}/runs/${run}/logs`,
    { token }
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

function encodeRequired(value: string | undefined, label: string): string {
  if (!value) {
    throw new Error(`Missing required parameter: ${label}`);
  }
  return encodeURIComponent(value);
}

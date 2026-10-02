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

import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
import type {
  AlertDeliveryListResponse,
  AlertEndpointPathParams,
  AlertEndpointRequest,
  AlertEndpointResponse,
  AlertTestResponse,
  ListAlertDeliveriesQuery,
  MonitorAlertConfigPathParams,
  MonitorAlertConfigRequest,
  MonitorAlertConfigResponse,
} from "@agent-management-platform/types";

function alertingBase(orgName: string | undefined) {
  return `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName ?? "default")}/alerting`;
}

function monitorAlertingPath(params: MonitorAlertConfigPathParams) {
  const { orgName = "default", projName, agentName, monitorName } = params;
  if (!projName || !agentName || !monitorName) {
    throw new Error("projName, agentName and monitorName are required");
  }
  return `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/monitors/${encodeURIComponent(monitorName)}/alerting`;
}

/**
 * Get the org alert endpoint. Resolves to null when none is configured.
 */
export async function getAlertEndpoint(
  params: AlertEndpointPathParams,
  getToken?: () => Promise<string>,
): Promise<AlertEndpointResponse | null> {
  const token = getToken ? await getToken() : undefined;
  try {
    const res = await httpGET(`${alertingBase(params.orgName)}/endpoint`, { token });
    return await res.json();
  } catch (err) {
    // httpGET throws on any non-2xx. A 404 means the org has no endpoint,
    // which is a valid state (never configured, or just deleted), not an error.
    if ((err as { status?: number })?.status === 404) return null;
    throw err;
  }
}

/**
 * Create or update the org alert endpoint.
 */
export async function upsertAlertEndpoint(
  params: AlertEndpointPathParams,
  body: AlertEndpointRequest,
  getToken?: () => Promise<string>,
): Promise<AlertEndpointResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(`${alertingBase(params.orgName)}/endpoint`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Delete the org alert endpoint.
 */
export async function deleteAlertEndpoint(
  params: AlertEndpointPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(`${alertingBase(params.orgName)}/endpoint`, { token });
  if (!res.ok) throw await res.json();
}

/**
 * Send a test alert to the org endpoint.
 */
export async function testAlertEndpoint(
  params: AlertEndpointPathParams,
  getToken?: () => Promise<string>,
): Promise<AlertTestResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${alertingBase(params.orgName)}/endpoint/test`, {}, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * List recent alert deliveries for the org.
 */
export async function listAlertDeliveries(
  params: AlertEndpointPathParams,
  query?: ListAlertDeliveriesQuery,
  getToken?: () => Promise<string>,
): Promise<AlertDeliveryListResponse> {
  const token = getToken ? await getToken() : undefined;
  const searchParams = query?.limit ? { limit: String(query.limit) } : undefined;
  const res = await httpGET(`${alertingBase(params.orgName)}/deliveries`, { searchParams, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Get a monitor's alert configuration.
 */
export async function getMonitorAlertConfig(
  params: MonitorAlertConfigPathParams,
  getToken?: () => Promise<string>,
): Promise<MonitorAlertConfigResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(monitorAlertingPath(params), { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

/**
 * Replace a monitor's alert configuration.
 */
export async function updateMonitorAlertConfig(
  params: MonitorAlertConfigPathParams,
  body: MonitorAlertConfigRequest,
  getToken?: () => Promise<string>,
): Promise<MonitorAlertConfigResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(monitorAlertingPath(params), body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

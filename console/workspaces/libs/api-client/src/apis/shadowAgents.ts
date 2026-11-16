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

import { httpDELETE, httpGET, httpPATCH, httpPOST, httpPUT, SERVICE_BASE } from "../utils";
import type {
  ShadowAgentSummary,
  ShadowConnectionListResponse,
  ShadowConnectionRequest,
  ShadowConnectionResponse,
  ShadowConnectionTest,
  ShadowFindingClearResponse,
  ShadowFindingDetail,
  ShadowFindingListQuery,
  ShadowFindingListResponse,
  ShadowFindingUpdateRequest,
  ShadowProviderListResponse,
  ShadowScanClearResponse,
  ShadowScanListResponse,
  ShadowScanRequest,
  ShadowScanResponse,
} from "@agent-management-platform/types";

async function tokenOf(getToken?: () => Promise<string>) {
  return getToken ? await getToken() : undefined;
}

/** The organization's Shadow Agent Discovery path. */
function shadowBase(orgName: string | undefined): string {
  return `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName ?? "default")}/shadow-agents`;
}

export async function listShadowProviders(
  orgName: string | undefined,
  getToken?: () => Promise<string>,
): Promise<ShadowProviderListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${shadowBase(orgName)}/providers`, { token });
  return res.json();
}

export async function getShadowSummary(
  orgName: string | undefined,
  getToken?: () => Promise<string>,
): Promise<ShadowAgentSummary> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${shadowBase(orgName)}/summary`, { token });
  return res.json();
}

export async function listShadowConnections(
  orgName: string | undefined,
  getToken?: () => Promise<string>,
): Promise<ShadowConnectionListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${shadowBase(orgName)}/connections`, { token });
  return res.json();
}

export async function getShadowConnection(
  orgName: string | undefined,
  connectionId: string,
  getToken?: () => Promise<string>,
): Promise<ShadowConnectionResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(
    `${shadowBase(orgName)}/connections/${encodeURIComponent(connectionId)}`,
    { token },
  );
  return res.json();
}

export async function createShadowConnection(
  orgName: string | undefined,
  body: ShadowConnectionRequest,
  getToken?: () => Promise<string>,
): Promise<ShadowConnectionResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(`${shadowBase(orgName)}/connections`, body, { token });
  return res.json();
}

export async function updateShadowConnection(
  orgName: string | undefined,
  connectionId: string,
  body: ShadowConnectionRequest,
  getToken?: () => Promise<string>,
): Promise<ShadowConnectionResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPUT(
    `${shadowBase(orgName)}/connections/${encodeURIComponent(connectionId)}`,
    body,
    { token },
  );
  return res.json();
}

export async function deleteShadowConnection(
  orgName: string | undefined,
  connectionId: string,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = await tokenOf(getToken);
  await httpDELETE(`${shadowBase(orgName)}/connections/${encodeURIComponent(connectionId)}`, {
    token,
  });
}

export async function testShadowConnection(
  orgName: string | undefined,
  connectionId: string,
  getToken?: () => Promise<string>,
): Promise<ShadowConnectionTest> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(
    `${shadowBase(orgName)}/connections/${encodeURIComponent(connectionId)}/test`,
    {},
    { token },
  );
  return res.json();
}

export async function listShadowScans(
  orgName: string | undefined,
  getToken?: () => Promise<string>,
): Promise<ShadowScanListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${shadowBase(orgName)}/scans`, { token });
  return res.json();
}

export async function getShadowScan(
  orgName: string | undefined,
  scanId: string,
  getToken?: () => Promise<string>,
): Promise<ShadowScanResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${shadowBase(orgName)}/scans/${encodeURIComponent(scanId)}`, {
    token,
  });
  return res.json();
}

export async function createShadowScan(
  orgName: string | undefined,
  body: ShadowScanRequest,
  getToken?: () => Promise<string>,
): Promise<ShadowScanResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(`${shadowBase(orgName)}/scans`, body, { token });
  return res.json();
}

/** Deletes the finished scans; a queued or running scan and all findings are kept. */
export async function clearShadowScans(
  orgName: string | undefined,
  getToken?: () => Promise<string>,
): Promise<ShadowScanClearResponse> {
  const token = await tokenOf(getToken);
  const res = await httpDELETE(`${shadowBase(orgName)}/scans`, { token });
  return res.json();
}

export async function listShadowFindings(
  orgName: string | undefined,
  query: ShadowFindingListQuery,
  getToken?: () => Promise<string>,
): Promise<ShadowFindingListResponse> {
  const token = await tokenOf(getToken);
  const searchParams: Record<string, string> = {};
  for (const [k, v] of Object.entries(query)) {
    if (v !== undefined && v !== "") searchParams[k] = String(v);
  }
  const res = await httpGET(`${shadowBase(orgName)}/findings`, { searchParams, token });
  return res.json();
}

export async function getShadowFinding(
  orgName: string | undefined,
  findingId: string,
  getToken?: () => Promise<string>,
): Promise<ShadowFindingDetail> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${shadowBase(orgName)}/findings/${encodeURIComponent(findingId)}`, {
    token,
  });
  return res.json();
}

/** Deletes an agent, on every connection that found it. */
export async function deleteShadowFinding(
  orgName: string | undefined,
  findingId: string,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = await tokenOf(getToken);
  await httpDELETE(`${shadowBase(orgName)}/findings/${encodeURIComponent(findingId)}`, { token });
}

/** Deletes every agent of the organization; platforms and scans are kept. */
export async function deleteShadowFindings(
  orgName: string | undefined,
  getToken?: () => Promise<string>,
): Promise<ShadowFindingClearResponse> {
  const token = await tokenOf(getToken);
  const res = await httpDELETE(`${shadowBase(orgName)}/findings`, { token });
  return res.json();
}

export async function updateShadowFinding(
  orgName: string | undefined,
  findingId: string,
  body: ShadowFindingUpdateRequest,
  getToken?: () => Promise<string>,
): Promise<ShadowFindingDetail> {
  const token = await tokenOf(getToken);
  const res = await httpPATCH(
    `${shadowBase(orgName)}/findings/${encodeURIComponent(findingId)}`,
    body,
    { token },
  );
  return res.json();
}

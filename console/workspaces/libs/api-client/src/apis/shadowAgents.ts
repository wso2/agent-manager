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

import { httpDELETE, httpGET, httpPOST, httpPOSTForm, httpPUT, SERVICE_BASE } from "../utils";
import type {
  ShadowAgentSummary,
  ShadowConnectionListResponse,
  ShadowConnectionRequest,
  ShadowConnectionResponse,
  ShadowConnectionTest,
  ShadowFindingClearResponse,
  ShadowFindingDetail,
  ShadowFindingHiddenRequest,
  ShadowFindingListQuery,
  ShadowFindingListResponse,
  ShadowInventoryFormat,
  ShadowInventoryUploadResponse,
  ShadowOnboardingRequest,
  ShadowProviderListResponse,
  ShadowRunnerScanRequest,
  ShadowRunnerScanResponse,
  ShadowScanClearResponse,
  ShadowScanListResponse,
  ShadowScanRequest,
  ShadowScanResponse,
  ShadowScanResults,
  ShadowScanResultsUploadResponse,
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

/** Creates a pending source code scan and returns the parts of the command that runs it. */
export async function createShadowRunnerScan(
  orgName: string | undefined,
  connectionId: string,
  body: ShadowRunnerScanRequest,
  getToken?: () => Promise<string>,
): Promise<ShadowRunnerScanResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(
    `${shadowBase(orgName)}/connections/${encodeURIComponent(connectionId)}/runner-scans`,
    body,
    { token },
  );
  return res.json();
}

/** Uploads the results of a source code scan that is waiting for them. */
export async function uploadShadowScanResults(
  orgName: string | undefined,
  scanId: string,
  body: ShadowScanResults,
  getToken?: () => Promise<string>,
): Promise<ShadowScanResultsUploadResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(
    `${shadowBase(orgName)}/scans/${encodeURIComponent(scanId)}/results`,
    body,
    { token },
  );
  return res.json();
}

/**
 * Cancels a scan that is waiting for results, queued or running; it becomes
 * Failed. Connections already finished keep their results; the others fail
 * with the message "cancelled". A finished scan gives 409.
 */
export async function cancelShadowScan(
  orgName: string | undefined,
  scanId: string,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = await tokenOf(getToken);
  await httpPOST(`${shadowBase(orgName)}/scans/${encodeURIComponent(scanId)}/cancel`, {}, {
    token,
  });
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

/** Links the agent to the external agent registered for it (on every connection). */
export async function setShadowFindingHidden(
  orgName: string | undefined,
  findingId: string,
  body: ShadowFindingHiddenRequest,
  getToken?: () => Promise<string>,
): Promise<ShadowFindingDetail> {
  const token = await tokenOf(getToken);
  const res = await httpPUT(
    `${shadowBase(orgName)}/findings/${encodeURIComponent(findingId)}/hidden`,
    body,
    { token },
  );
  return res.json();
}

export async function onboardShadowFinding(
  orgName: string | undefined,
  findingId: string,
  body: ShadowOnboardingRequest,
  getToken?: () => Promise<string>,
): Promise<ShadowFindingDetail> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(
    `${shadowBase(orgName)}/findings/${encodeURIComponent(findingId)}/onboarding`,
    body,
    { token },
  );
  return res.json();
}

/** The default file name of each inventory template. */
const INVENTORY_TEMPLATE_FILES: Record<ShadowInventoryFormat, string> = {
  json: "agent-inventory-template.json",
  csv: "agent-inventory-template.csv",
  xlsx: "agent-inventory-template.xlsx",
};

/** The file name in a Content-Disposition header, if it gives one. */
export function contentDispositionFileName(header: string | null): string | undefined {
  if (!header) return undefined;
  const star = /filename\*\s*=\s*(?:UTF-8'[^']*')?([^;]+)/i.exec(header);
  if (star) {
    try {
      return decodeURIComponent(star[1].trim().replace(/^"|"$/g, ""));
    } catch {
      // Fall back to the plain filename.
    }
  }
  const plain = /filename\s*=\s*("([^"]*)"|[^;]+)/i.exec(header);
  const name = (plain?.[2] ?? plain?.[1])?.trim();
  return name || undefined;
}

/** Fetches the inventory template in a format: the file and its name. */
export async function fetchShadowInventoryTemplate(
  orgName: string | undefined,
  format: ShadowInventoryFormat,
  getToken?: () => Promise<string>,
): Promise<{ blob: Blob; fileName: string }> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${shadowBase(orgName)}/inventory/template`, {
    searchParams: { format },
    token,
  });
  const fileName =
    contentDispositionFileName(res.headers.get("Content-Disposition")) ??
    INVENTORY_TEMPLATE_FILES[format];
  return { blob: await res.blob(), fileName };
}

/** Saves a file in the browser's downloads. */
export function saveBlob(blob: Blob, fileName: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = fileName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/** Downloads the inventory template in a format (it needs the auth token, so it is fetched). */
export async function downloadShadowInventoryTemplate(
  orgName: string | undefined,
  format: ShadowInventoryFormat,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { blob, fileName } = await fetchShadowInventoryTemplate(orgName, format, getToken);
  saveBlob(blob, fileName);
}

/**
 * Uploads an inventory file (JSON, CSV or Excel) to a Bring your own inventory
 * connection. Its agents replace the connection's; agents not in the file are
 * marked gone.
 */
export async function uploadShadowInventory(
  orgName: string | undefined,
  connectionId: string,
  file: File,
  getToken?: () => Promise<string>,
): Promise<ShadowInventoryUploadResponse> {
  const token = await tokenOf(getToken);
  const form = new FormData();
  form.append("file", file, file.name);
  const res = await httpPOSTForm(
    `${shadowBase(orgName)}/connections/${encodeURIComponent(connectionId)}/inventory`,
    form,
    { token },
  );
  return res.json();
}

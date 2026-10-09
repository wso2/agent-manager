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
  WebhookDeliveryListResponse,
  WebhookEventTypeListResponse,
  WebhookListResponse,
  WebhookRequest,
  WebhookResponse,
  WebhookScope,
  WebhookSecretResponse,
  WebhookTargetParams,
  WebhookTestResponse,
} from "@agent-management-platform/types";

/** Base path of a scope's webhooks. */
export function webhooksBase(target: WebhookTargetParams): string {
  const org = `${SERVICE_BASE}/orgs/${encodeURIComponent(target.orgName ?? "default")}`;
  if (target.scope === "org") return `${org}/webhooks`;
  if (!target.projName) throw new Error("projName is required");
  const project = `${org}/projects/${encodeURIComponent(target.projName)}`;
  if (target.scope === "project") return `${project}/webhooks`;
  if (!target.agentName) throw new Error("agentName is required");
  return `${project}/agents/${encodeURIComponent(target.agentName)}/webhooks`;
}

async function tokenOf(getToken?: () => Promise<string>) {
  return getToken ? await getToken() : undefined;
}

export async function listWebhookEventTypes(
  orgName: string | undefined,
  scope: WebhookScope,
  getToken?: () => Promise<string>,
): Promise<WebhookEventTypeListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName ?? "default")}/webhook-event-types`,
    { searchParams: { scope }, token },
  );
  return res.json();
}

export async function listWebhooks(
  target: WebhookTargetParams,
  getToken?: () => Promise<string>,
): Promise<WebhookListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(webhooksBase(target), { token });
  return res.json();
}

export async function createWebhook(
  target: WebhookTargetParams,
  body: WebhookRequest,
  getToken?: () => Promise<string>,
): Promise<WebhookResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(webhooksBase(target), body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateWebhook(
  target: WebhookTargetParams,
  id: string,
  body: WebhookRequest,
  getToken?: () => Promise<string>,
): Promise<WebhookResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPUT(`${webhooksBase(target)}/${encodeURIComponent(id)}`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteWebhook(
  target: WebhookTargetParams,
  id: string,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = await tokenOf(getToken);
  const res = await httpDELETE(`${webhooksBase(target)}/${encodeURIComponent(id)}`, { token });
  if (!res.ok) throw await res.json();
}

export async function rotateWebhookSecret(
  target: WebhookTargetParams,
  id: string,
  getToken?: () => Promise<string>,
): Promise<WebhookSecretResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(`${webhooksBase(target)}/${encodeURIComponent(id)}/rotate-secret`, {}, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function testWebhook(
  target: WebhookTargetParams,
  id: string,
  getToken?: () => Promise<string>,
): Promise<WebhookTestResponse> {
  const token = await tokenOf(getToken);
  const res = await httpPOST(`${webhooksBase(target)}/${encodeURIComponent(id)}/test`, {}, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function listWebhookDeliveries(
  target: WebhookTargetParams,
  id: string,
  limit?: number,
  getToken?: () => Promise<string>,
): Promise<WebhookDeliveryListResponse> {
  const token = await tokenOf(getToken);
  const res = await httpGET(`${webhooksBase(target)}/${encodeURIComponent(id)}/deliveries`, {
    searchParams: limit ? { limit: String(limit) } : undefined,
    token,
  });
  return res.json();
}

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

import { httpDELETE, httpGET, httpPOST, httpPUT, moduleBase } from "../../shared/utils";
import {
  SUBSCRIPTION_PLAN_API_VERSION,
  SUBSCRIPTION_PLAN_KIND,
  type CreateSubscriptionPlanRequest,
  type ListSubscriptionPlansPathParams,
  type ListSubscriptionPlansQuery,
  type SubscriptionPlan,
  type SubscriptionPlanDocument,
  type SubscriptionPlanDocumentList,
  type SubscriptionPlanListResponse,
  type SubscriptionPlanPathParams,
  type SubscriptionPlanSpec,
  type UpdateSubscriptionPlanRequest,
} from "@agent-management-platform/types";

// The backend `ai-hub` module's routes. The organization comes from the caller's token, so
// the path has none.
const PLANS_BASE = `${moduleBase("ai-hub")}/subscription-plans`;

// The API serves each plan as a document; the UI works on a flat plan. These convert between them.

export function planFromDocument(doc: SubscriptionPlanDocument): SubscriptionPlan {
  const { metadata, spec, status } = doc;
  return {
    id: metadata.name,
    uid: metadata.uid,
    displayName: spec.displayName,
    description: spec.description,
    conditions: spec.conditions ?? [],
    expiryTime: spec.expiryTime ?? undefined,
    status: spec.status ?? "ACTIVE",
    createdBy: metadata.createdBy,
    createdAt: metadata.creationTimestamp,
    updatedAt: metadata.updateTimestamp,
    usage: status?.usage,
    origin: metadata.origin,
    deprecated: status?.deprecated === true,
    deprecatedAt: status?.deprecationTimestamp,
  };
}

export function planListFromDocuments(
  docs: SubscriptionPlanDocumentList,
): SubscriptionPlanListResponse {
  return { list: docs.list.map(planFromDocument), count: docs.count, pagination: docs.pagination };
}

/** A plan document to send: metadata.name is the handle, the rest is spec. */
export function documentFromPlan(
  name: string,
  spec: SubscriptionPlanSpec,
): SubscriptionPlanDocument {
  return {
    apiVersion: SUBSCRIPTION_PLAN_API_VERSION,
    kind: SUBSCRIPTION_PLAN_KIND,
    metadata: { name },
    spec,
  };
}

async function throwHttpError(res: Response): Promise<never> {
  const body = await res.json().catch(() => undefined);
  const message = (body as { message?: string } | undefined)?.message;
  const err = new Error(message || `Request failed: ${res.status} ${res.statusText}`);
  throw Object.assign(err, { status: res.status, body });
}

export async function listSubscriptionPlans(
  _params: ListSubscriptionPlansPathParams,
  query?: ListSubscriptionPlansQuery,
  getToken?: () => Promise<string>,
): Promise<SubscriptionPlanListResponse> {
  const search: Record<string, string> = {};
  for (const [key, value] of Object.entries(query ?? {})) {
    if (value !== undefined && value !== "") search[key] = String(value);
  }
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(PLANS_BASE, { searchParams: search, token });
  if (!res.ok) await throwHttpError(res);
  return planListFromDocuments(await res.json());
}

export async function getSubscriptionPlan(
  params: SubscriptionPlanPathParams,
  getToken?: () => Promise<string>,
): Promise<SubscriptionPlan> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${PLANS_BASE}/${encodeURIComponent(params.planId)}`, { token });
  if (!res.ok) await throwHttpError(res);
  return planFromDocument(await res.json());
}

export async function createSubscriptionPlan(
  _params: ListSubscriptionPlansPathParams,
  body: CreateSubscriptionPlanRequest,
  getToken?: () => Promise<string>,
): Promise<SubscriptionPlan> {
  const token = getToken ? await getToken() : undefined;
  const { id, ...spec } = body;
  const res = await httpPOST(PLANS_BASE, documentFromPlan(id, spec), { token });
  return planFromDocument(await res.json());
}

export async function updateSubscriptionPlan(
  params: SubscriptionPlanPathParams,
  body: UpdateSubscriptionPlanRequest,
  getToken?: () => Promise<string>,
): Promise<SubscriptionPlan> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${PLANS_BASE}/${encodeURIComponent(params.planId)}`,
    documentFromPlan(params.planId, body),
    { token },
  );
  return planFromDocument(await res.json());
}

/**
 * Deprecates a plan (DELETE is a soft delete): it stays, read only, for the subscriptions on
 * it. Returns the deprecated plan.
 */
export async function deprecateSubscriptionPlan(
  params: SubscriptionPlanPathParams,
  getToken?: () => Promise<string>,
): Promise<SubscriptionPlan> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(`${PLANS_BASE}/${encodeURIComponent(params.planId)}`, { token });
  if (!res.ok) await throwHttpError(res);
  return planFromDocument(await res.json());
}

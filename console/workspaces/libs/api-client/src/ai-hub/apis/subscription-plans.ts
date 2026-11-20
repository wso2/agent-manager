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
import type {
  CreateSubscriptionPlanRequest,
  ListSubscriptionPlansPathParams,
  ListSubscriptionPlansQuery,
  SubscriptionPlan,
  SubscriptionPlanListResponse,
  SubscriptionPlanPathParams,
  UpdateSubscriptionPlanRequest,
} from "@agent-management-platform/types";

// The backend `ai-hub` module's routes. The organization comes from the caller's token, so
// the path has none.
const PLANS_BASE = `${moduleBase("ai-hub")}/subscription-plans`;

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
  return res.json();
}

export async function getSubscriptionPlan(
  params: SubscriptionPlanPathParams,
  getToken?: () => Promise<string>,
): Promise<SubscriptionPlan> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${PLANS_BASE}/${encodeURIComponent(params.planId)}`, { token });
  if (!res.ok) await throwHttpError(res);
  return res.json();
}

export async function createSubscriptionPlan(
  _params: ListSubscriptionPlansPathParams,
  body: CreateSubscriptionPlanRequest,
  getToken?: () => Promise<string>,
): Promise<SubscriptionPlan> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(PLANS_BASE, body, { token });
  return res.json();
}

export async function updateSubscriptionPlan(
  params: SubscriptionPlanPathParams,
  body: UpdateSubscriptionPlanRequest,
  getToken?: () => Promise<string>,
): Promise<SubscriptionPlan> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(`${PLANS_BASE}/${encodeURIComponent(params.planId)}`, body, { token });
  return res.json();
}

export async function deleteSubscriptionPlan(
  params: SubscriptionPlanPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const token = getToken ? await getToken() : undefined;
  await httpDELETE(`${PLANS_BASE}/${encodeURIComponent(params.planId)}`, { token });
}

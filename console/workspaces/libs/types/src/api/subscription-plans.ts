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

// Subscription plans, served by the backend `aihub` module at
// /m/aihub/subscription-plans (GET/POST /subscription-plans, GET/PUT/DELETE
// /subscription-plans/{subscriptionPlanId}).

export type SubscriptionPlanStatus = "ACTIVE" | "INACTIVE";

export type SubscriptionPlanConditionType =
  | "request-count"
  | "tool-call-count"
  | "token-count"
  | "custom";

/** One condition of a plan. A built-in type has a fixed id and SubscriptionPlanConditionLimits as
 * params; a custom one has its own id and any JSON object. */
export interface SubscriptionPlanCondition {
  id?: string;
  type: SubscriptionPlanConditionType;
  params: Record<string, unknown>;
}

export interface SubscriptionPlanConditionWindow {
  count: number;
  timeAmount?: number;
  timeUnit: "MINUTE" | "HOUR" | "DAY" | "MONTH";
}

export interface SubscriptionPlanConditionLimits {
  limits: SubscriptionPlanConditionWindow[];
  onExceed?: "reject" | "allow";
  tokenType?: "total" | "prompt" | "completion";
}

/** What offers or uses a plan. A plan in use keeps its name and conditions and can't be deleted. */
export interface SubscriptionPlanUsage {
  inUse: boolean;
  restApis: number;
  mcpProxies: number;
  drafts: number;
  publications: number;
  subscriptions: number;
}

export interface SubscriptionPlan {
  /** The plan's handle, immutable after creation. */
  id: string;
  displayName: string;
  description?: string;
  conditions: SubscriptionPlanCondition[];
  expiryTime?: string;
  organizationId?: string;
  status: SubscriptionPlanStatus;
  createdBy?: string;
  updatedBy?: string;
  createdAt?: string;
  updatedAt?: string;
  /** Only in GET of one plan. */
  usage?: SubscriptionPlanUsage;
}

export interface SubscriptionPlanListResponse {
  list: SubscriptionPlan[];
  count: number;
  pagination: { total: number; offset: number; limit: number };
}

export interface CreateSubscriptionPlanRequest {
  id: string;
  displayName: string;
  description?: string;
  conditions?: SubscriptionPlanCondition[];
  expiryTime?: string;
  status?: SubscriptionPlanStatus;
}

/** PUT body. displayName is required; an omitted field keeps the stored value, and
 * `expiryTime: null` clears the expiry. */
export interface UpdateSubscriptionPlanRequest {
  id?: string;
  displayName: string;
  description?: string;
  conditions?: SubscriptionPlanCondition[];
  expiryTime?: string | null;
  status?: SubscriptionPlanStatus;
}

export interface ListSubscriptionPlansPathParams {
  orgName?: string;
}

export interface ListSubscriptionPlansQuery {
  limit?: number;
  offset?: number;
  /** Case-insensitive match on the name or handle. */
  query?: string;
  status?: SubscriptionPlanStatus;
}

export interface SubscriptionPlanPathParams {
  orgName?: string;
  planId: string;
}

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

// Subscription plans, served by the backend `ai-hub` module at
// /m/ai-hub/subscription-plans (GET/POST /subscription-plans, GET/PUT/DELETE
// /subscription-plans/{subscriptionPlanId}; DELETE deprecates the plan, a soft delete). The
// API serves each plan as a document (SubscriptionPlanDocument); the UI works on the flat
// SubscriptionPlan, and the API client converts between them.

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

/**
 * What offers or uses a plan. A plan in use (one with subscriptions) keeps its name, conditions
 * and expiry; an offer alone doesn't lock it.
 */
export interface SubscriptionPlanUsage {
  inUse: boolean;
  mcpProxies: number;
  subscriptions: number;
}

/** A plan as the UI works on it (the document, flattened). */
export interface SubscriptionPlan {
  /** The plan's handle (metadata.name), immutable after creation. */
  id: string;
  /** The server-assigned ID (metadata.uid). */
  uid?: string;
  displayName: string;
  description?: string;
  conditions: SubscriptionPlanCondition[];
  expiryTime?: string;
  status: SubscriptionPlanStatus;
  createdBy?: string;
  createdAt?: string;
  updatedAt?: string;
  /** Only in GET of one plan (status.usage). */
  usage?: SubscriptionPlanUsage;
  /**
   * Where the plan was made (metadata.origin): "ai-control-plane", or the developer portal
   * that copied it in.
   */
  origin?: string;
  /**
   * A deprecated plan (status.deprecated) is read only: it stays for the subscriptions on it,
   * but can't be changed, offered on an MCP server or subscribed to again.
   */
  deprecated: boolean;
  /** When the plan was deprecated (status.deprecationTimestamp). */
  deprecatedAt?: string;
}

export const SUBSCRIPTION_PLAN_API_VERSION = "ai-control-plane.choreolabs.com/v1";
export const SUBSCRIPTION_PLAN_KIND = "SubscriptionPlan";

export interface SubscriptionPlanDocumentMetadata {
  /** The plan's handle. */
  name: string;
  uid?: string;
  creationTimestamp?: string;
  updateTimestamp?: string;
  createdBy?: string;
  /** Where the plan was made; set by the server. */
  origin?: string;
}

export interface SubscriptionPlanSpec {
  displayName: string;
  description?: string;
  status?: SubscriptionPlanStatus;
  /** null clears the expiry on update. */
  expiryTime?: string | null;
  conditions?: SubscriptionPlanCondition[];
}

/** A plan as the API serves it: a document, as Kubernetes resources are. */
export interface SubscriptionPlanDocument {
  apiVersion: typeof SUBSCRIPTION_PLAN_API_VERSION;
  kind: typeof SUBSCRIPTION_PLAN_KIND;
  metadata: SubscriptionPlanDocumentMetadata;
  spec: SubscriptionPlanSpec;
  /** Observed by the server, ignored on input. */
  status?: {
    deprecated?: boolean;
    deprecationTimestamp?: string;
    usage?: SubscriptionPlanUsage;
  };
}

export interface SubscriptionPlanDocumentList {
  list: SubscriptionPlanDocument[];
  count: number;
  pagination: { total: number; offset: number; limit: number };
}

export interface SubscriptionPlanListResponse {
  list: SubscriptionPlan[];
  count: number;
  pagination: { total: number; offset: number; limit: number };
}

/** A new plan; the API client sends it as a document. */
export interface CreateSubscriptionPlanRequest {
  id: string;
  displayName: string;
  description?: string;
  conditions?: SubscriptionPlanCondition[];
  expiryTime?: string;
  status?: SubscriptionPlanStatus;
}

/** A change to a plan; the API client sends it as a document named by the plan's id.
 * displayName is required; an omitted field keeps the stored value, and `expiryTime: null`
 * clears the expiry. */
export interface UpdateSubscriptionPlanRequest {
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

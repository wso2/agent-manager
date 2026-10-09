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

export type WebhookScope = 'org' | 'project' | 'agent';

export interface WebhookEventType {
  type: string;
  scope: WebhookScope;
  category: string;
  description: string;
}

export interface WebhookEventTypeListResponse {
  eventTypes: WebhookEventType[];
}

export interface WebhookRequest {
  name: string;
  description?: string;
  url: string;
  /**
   * Environments an agent webhook receives events of; at least one is
   * required for agent webhooks. Agent events not tied to an environment,
   * such as builds, go to every agent webhook.
   */
  environments?: string[];
  eventTypes: string[];
  enabled?: boolean;
}

export interface WebhookResponse {
  id: string;
  scope: WebhookScope;
  name: string;
  description: string;
  url: string;
  environments: string[];
  eventTypes: string[];
  enabled: boolean;
  /** Present only in the response that created the webhook. */
  signingSecret?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WebhookListResponse {
  webhooks: WebhookResponse[];
}

export interface WebhookSecretResponse {
  signingSecret: string;
}

export interface WebhookTestResponse {
  delivered: boolean;
  statusCode?: number;
  error?: string;
}

export type WebhookDeliveryStatus = 'pending' | 'delivered' | 'failed';

export interface WebhookDeliveryResponse {
  id: string;
  eventId: string;
  eventType: string;
  status: WebhookDeliveryStatus;
  attempts: number;
  responseCode?: number;
  lastError?: string;
  deliveredAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WebhookDeliveryListResponse {
  deliveries: WebhookDeliveryResponse[];
}

/**
 * Identifies the owner of a set of webhooks: the org, a project in it, or an
 * agent in a project.
 */
export interface WebhookTargetParams {
  scope: WebhookScope;
  orgName?: string;
  projName?: string;
  agentName?: string;
}

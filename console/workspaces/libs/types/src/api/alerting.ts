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

import type { OrgPathParams } from './common';
import type { MonitorPathParams } from './monitors';

// Org alert endpoint
export interface AlertEndpointRequest {
  url: string;
  enabled?: boolean;
  /** Omit to keep stored headers; pass {} to clear them. */
  headers?: Record<string, string>;
  regenerateSigningSecret?: boolean;
}

export interface AlertEndpointResponse {
  url: string;
  enabled: boolean;
  headerNames: string[];
  /** Present only in the response that generated it. */
  signingSecret?: string;
  consecutiveFailures: number;
  lastSuccessAt?: string;
  lastFailureAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AlertTestResponse {
  delivered: boolean;
  statusCode?: number;
  error?: string;
}

export type AlertDeliveryStatus = 'pending' | 'sent' | 'dead';

export interface AlertDeliveryResponse {
  id: string;
  eventId: string;
  eventType: string;
  monitorName?: string;
  status: AlertDeliveryStatus;
  attempts: number;
  lastResponseCode?: number;
  lastError?: string;
  nextAttemptAt?: string;
  deliveredAt?: string;
  createdAt: string;
}

export interface AlertDeliveryListResponse {
  deliveries: AlertDeliveryResponse[];
}

// Monitor alert config
export type MonitorAlertOperator = 'lt' | 'lte';

export interface MonitorAlertThreshold {
  /** Display name of one of the monitor's evaluators. */
  evaluator: string;
  aggregation: string;
  operator?: MonitorAlertOperator;
  value: number;
}

export interface MonitorAlertConfigRequest {
  enabled: boolean;
  alertOnRunFailure?: boolean;
  thresholds?: MonitorAlertThreshold[];
  cooldownMinutes?: number;
  consecutiveBreaches?: number;
}

export interface MonitorAlertConfigResponse {
  enabled: boolean;
  alertOnRunFailure: boolean;
  thresholds: MonitorAlertThreshold[];
  cooldownMinutes: number;
  consecutiveBreaches: number;
  orgEndpointConfigured: boolean;
  lastAlertedAt?: string;
}

// Path params
export type AlertEndpointPathParams = OrgPathParams;
export type MonitorAlertConfigPathParams = MonitorPathParams;

export interface ListAlertDeliveriesQuery {
  limit?: number;
}

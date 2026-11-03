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

/** Which of a run's aggregated scores a threshold compares. */
export type ScoreThresholdAggregation = 'mean' | 'pass_rate';

export type ScoreThresholdOperator = 'below' | 'above';

/**
 * Sends alerts when a score of the monitor's runs crosses a value,
 * and again when it crosses back.
 */
export interface MonitorScoreThreshold {
  /** Display name of one of the monitor's evaluators. */
  evaluator: string;
  aggregation: ScoreThresholdAggregation;
  operator: ScoreThresholdOperator;
  /** Between 0 and 1. */
  value: number;
}

export interface MonitorAlertRequest {
  destinationIds: string[];
  /** Notify when a run fails or cannot start. */
  notifyOnFailure: boolean;
  /** Notify when a run completes. */
  notifyOnSuccess: boolean;
  thresholds: MonitorScoreThreshold[];
}

export interface MonitorAlertResponse extends MonitorAlertRequest {
  /** Whether the caller alertes the monitor. */
  enabled: boolean;
  /** The thresholds the latest evaluated run breached. */
  breached?: MonitorScoreThreshold[];
  updatedAt?: string;
}

export interface MonitorAlertPathParams {
  orgName: string | undefined;
  projName: string | undefined;
  agentName: string | undefined;
  monitorName: string | undefined;
}

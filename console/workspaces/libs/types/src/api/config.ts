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

// Response for the unauthenticated runtime discovery endpoint served by
// agent-manager: GET /api/v1/config.
export interface ConfigResponse {
  observerBaseUrl: string;
  /**
   * Whether this deployment accepts console usage analytics, from the
   * service's CONSOLE_ANALYTICS_ENABLED. The console has no flag of its own —
   * that one switch governs the whole path. Absent is treated as false.
   */
  consoleAnalyticsEnabled?: boolean;
}

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

import { httpGET, SERVICE_BASE } from "../../shared/utils";
import { globalConfig, type ConfigResponse } from "@agent-management-platform/types";

// The whole app renders a full-page loader until this resolves, so bound the
// request: a hung config endpoint must surface as an error (via AbortError)
// rather than trapping the user on the loader forever.
const CONFIG_FETCH_TIMEOUT_MS = 5000;

/**
 * Unauthenticated discovery endpoint served by agent-manager.
 *
 * Targets configDiscoveryBaseUrl when set (the public …-agent-manager-config-api
 * host on gateway deployments), else falls back to apiBaseUrl (direct
 * deployments with no gateway). Trailing slashes are stripped so httpGET's
 * `${baseUrl}${context}` join yields exactly one separator rather than
 * "…//api/v1/config".
 */
export async function getRuntimeConfig(): Promise<ConfigResponse> {
  const baseUrl = (
    globalConfig.configDiscoveryBaseUrl?.trim() || globalConfig.apiBaseUrl
  ).replace(/\/+$/, "");
  const res = await httpGET(`${SERVICE_BASE}/config`, {
    baseUrl,
    timeoutMs: CONFIG_FETCH_TIMEOUT_MS,
  });
  return res.json();
}

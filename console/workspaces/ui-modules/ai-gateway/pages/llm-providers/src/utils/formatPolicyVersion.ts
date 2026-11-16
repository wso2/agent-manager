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

/**
 * Normalizes to exactly one "v" prefix regardless of source convention: hub-sourced
 * versions were historically bare semver (e.g. "1.0.1"), gateway-manifest-sourced
 * versions already carry a "v" (e.g. "v1.0.1") — without this, the latter renders
 * as a doubled "vv1.0.1".
 */
export function formatPolicyVersion(version: string): string {
  return `v${version.replace(/^v/i, "")}`;
}

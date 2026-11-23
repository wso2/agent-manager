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

/** `v1.0.2` -> `v1`; anything that is not a numeric version is returned as given. */
export function normalizePolicyVersionToMajor(version: string): string {
  const trimmedVersion = version.trim();
  if (!trimmedVersion) return trimmedVersion;

  let versionWithoutPrefix = trimmedVersion;
  if (versionWithoutPrefix.toLowerCase().startsWith("v")) {
    versionWithoutPrefix = versionWithoutPrefix.slice(1);
  }
  if (!versionWithoutPrefix) return trimmedVersion;

  let majorVersion = versionWithoutPrefix;
  const dotIndex = majorVersion.indexOf(".");
  if (dotIndex >= 0) majorVersion = majorVersion.slice(0, dotIndex);
  const dashIndex = majorVersion.indexOf("-");
  if (dashIndex >= 0) majorVersion = majorVersion.slice(0, dashIndex);
  majorVersion = majorVersion.trim();
  if (!/^\d+$/.test(majorVersion)) return trimmedVersion;
  return `v${majorVersion}`;
}

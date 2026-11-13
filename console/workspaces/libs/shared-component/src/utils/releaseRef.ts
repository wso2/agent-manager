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

import { globalConfig } from "@agent-management-platform/types";

/**
 * The configured Agent Manager release version (e.g. "v0.15.0"). Empty when
 * not injected (dev), which falls back to `main`.
 */
export function getAmpVersion(): string {
  return globalConfig.ampVersion?.trim() || "";
}

/**
 * Git ref of this release in the Agent Manager repository, for links to files
 * published from it. Release tags are `amp/vX.Y.Z`, so a versioned build pins to
 * that tag. An unset or dev placeholder version (e.g. `0.0.0-dev`) has no
 * matching tag, so fall back to `main`. The leading "v" is normalized so both
 * "v0.15.0" and "0.15.0" work.
 */
export function getScriptRef(): string {
  const version = getAmpVersion();
  if (!version || version.includes("dev")) return "main";
  const bare = version.startsWith("v") ? version.slice(1) : version;
  return `amp/v${bare}`;
}

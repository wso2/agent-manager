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

import { readHeaderBreadcrumbsState } from "@wso2/oxygen-ui";

// Level ids of the top bar's breadcrumb trail. oxygen-ui persists the selected
// org and project under these keys, so readers must use the same ones.
export const ORG_LEVEL = "organization";
export const PROJECT_LEVEL = "project";
export const AGENT_LEVEL = "agent";

/** The project the top bar last remembered for this org, if any. */
export function readRememberedProject(orgId?: string): string | undefined {
  try {
    const stored = readHeaderBreadcrumbsState()?.value;
    return stored?.[ORG_LEVEL] === orgId ? (stored?.[PROJECT_LEVEL] ?? undefined) : undefined;
  } catch {
    return undefined;
  }
}

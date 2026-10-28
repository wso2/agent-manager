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

import { generatePath } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

/** Configure Agent tab link; mirrors CONFIGURE_TAB_PARAM/CONFIGURE_ENV_PARAM (configureTabs.ts). */
export function buildConfigureTabHref(
  orgId: string,
  projectId: string,
  agentId: string,
  tab: "llm" | "tools" | "a2a",
  envName?: string,
): string {
  const path = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.configure.path,
    { orgId, projectId, agentId },
  );
  const query = new URLSearchParams({ tab });
  if (envName) query.set("env", envName);
  return `${path}?${query}`;
}

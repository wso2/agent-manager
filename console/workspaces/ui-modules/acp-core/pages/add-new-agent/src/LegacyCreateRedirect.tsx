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

import { generatePath, Navigate, useLocation, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

/**
 * /newAgent/create was the "Source Code / Agent Catalog" step, now replaced by
 * agent-type cards on the Add a New Agent page. Old links and history entries
 * land here; send them to that page, keeping the wizard's return state.
 */
export function LegacyCreateRedirect() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId?: string }>();
  const location = useLocation();
  return (
    <Navigate
      to={generatePath(absoluteRouteMap.children.org.children.projects.children.newAgent.path, {
        orgId: orgId ?? "",
        projectId: projectId ?? "default",
      })}
      state={location.state}
      replace
    />
  );
}

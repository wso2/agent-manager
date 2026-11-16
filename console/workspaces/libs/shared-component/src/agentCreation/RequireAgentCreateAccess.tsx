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

import type { ReactNode } from "react";
import { generatePath, Navigate, useParams } from "react-router-dom";
import { useFeatureAccess } from "@agent-management-platform/feature-management";
import { absoluteRouteMap } from "@agent-management-platform/types";

/**
 * Wraps an agent type's create form: without the agent_create permission the
 * user is sent back to the project, as the "Add a New Agent" page does. Each
 * module's create route needs it, since the forms are separate pages.
 */
export function RequireAgentCreateAccess({ children }: { children: ReactNode }) {
  const canCreate = useFeatureAccess("agent_create");
  const { orgId, projectId } = useParams<{ orgId: string; projectId?: string }>();
  if (!canCreate) {
    return (
      <Navigate
        to={generatePath(absoluteRouteMap.children.org.children.projects.path, {
          orgId: orgId ?? "",
          projectId: projectId ?? "default",
        })}
        replace
      />
    );
  }
  return <>{children}</>;
}

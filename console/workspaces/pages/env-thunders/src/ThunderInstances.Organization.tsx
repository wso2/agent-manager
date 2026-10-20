/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { Navigate, Route, Routes, generatePath, useParams } from "react-router-dom";
import { absoluteRouteMap, isEnvironmentManagedMode } from "@agent-management-platform/types";
import { AgentIdentityEnvironmentProvider } from "./context/AgentIdentityEnvironmentContext";
import { AgentsOrganization } from "./AgentsOrganization";
import { RolesOrganization } from "./RolesOrganization";
import { GroupsOrganization } from "./GroupsOrganization";

const thunderInstancesNode = absoluteRouteMap.children.org.children.thunderInstances;

// Back-compat redirect: agent/role/group management moved from
// /thunder-instances/view/:envName/(agents|roles|groups) to the top-level
// /thunder-instances/(agents|roles|groups) pages, which read the environment
// from an `envName` query param instead of the URL path. The old
// view/:envName overview (gateways, identity provider) moved to
// /environments/:envName. Preserve any trailing sub-path so existing deep
// links keep working.
function LegacyThunderInstanceViewRedirect() {
  const { orgId, envName, "*": rest } = useParams<{
    orgId: string;
    envName: string;
    "*": string;
  }>();
  const [section, ...tail] = (rest ?? "").split("/").filter(Boolean);

  if (section === "agents" || section === "roles" || section === "groups") {
    const base = generatePath(thunderInstancesNode.children[section].path, { orgId });
    const target = tail.length > 0 ? `${base}/${tail.join("/")}` : base;
    return <Navigate to={`${target}?envName=${encodeURIComponent(envName ?? "")}`} replace />;
  }

  if (isEnvironmentManagedMode()) {
    const base = generatePath(thunderInstancesNode.children.agents.path, { orgId });
    return <Navigate to={`${base}?envName=${encodeURIComponent(envName ?? "")}`} replace />;
  }

  return (
    <Navigate
      to={generatePath(
        absoluteRouteMap.children.org.children.environments.children.view.path,
        { orgId, envName },
      )}
      replace
    />
  );
}

export const ThunderInstancesOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();

  return (
    <AgentIdentityEnvironmentProvider>
      <Routes>
        <Route path="view/:envName/*" element={<LegacyThunderInstanceViewRedirect />} />
        <Route path="agents/*" element={<AgentsOrganization />} />
        <Route path="roles/*" element={<RolesOrganization />} />
        <Route path="groups/*" element={<GroupsOrganization />} />
        <Route
          path="*"
          element={
            <Navigate
              to={generatePath(
                isEnvironmentManagedMode()
                  ? thunderInstancesNode.children.agents.path
                  : absoluteRouteMap.children.org.children.environments.path,
                { orgId },
              )}
              replace
            />
          }
        />
      </Routes>
    </AgentIdentityEnvironmentProvider>
  );
};

export default ThunderInstancesOrganization;

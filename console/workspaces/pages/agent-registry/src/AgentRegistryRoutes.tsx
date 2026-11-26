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

import { generatePath, Navigate, Route, Routes, useParams } from "react-router-dom";
import { absoluteRouteMap, relativeRouteMap } from "@agent-management-platform/types";
import { AgentRegistry } from "./AgentRegistry/AgentRegistry";
import { AgentRegistryOverview } from "./AgentRegistryOverview/AgentRegistryOverview";
import {
  UnmanagedAgentOnboardPage,
  UnmanagedAgentPage,
} from "./UnmanagedAgent/UnmanagedAgentPages";

const registryRoutes = relativeRouteMap.children.org.children.agentRegistry.children;
const unmanagedRoute = registryRoutes.unmanagedAgent;
const unmanagedAgentAbsolutePath =
  absoluteRouteMap.children.org.children.agentRegistry.children.unmanagedAgent.path;

/** A link from before the rename (/agent-registry/unregistered/...): the same page, renamed. */
function RedirectFromUnregistered() {
  const { orgId = "", findingId = "", "*": rest = "" } = useParams();
  const page = generatePath(unmanagedAgentAbsolutePath, { orgId, findingId });
  return <Navigate to={rest ? `${page}/${rest}` : page} replace />;
}

/**
 * The registry's pages under /agent-registry: the list, a managed agent's overview, and an
 * unmanaged agent's page with onboarding.
 */
export function AgentRegistryRoutes() {
  return (
    <Routes>
      <Route index element={<AgentRegistry />} />
      <Route path={registryRoutes.agent.path} element={<AgentRegistryOverview />} />
      <Route path={unmanagedRoute.path} element={<UnmanagedAgentPage />} />
      <Route
        path={`${unmanagedRoute.path}/${unmanagedRoute.children.onboard.path}`}
        element={<UnmanagedAgentOnboardPage />}
      />
      <Route path="unregistered/:findingId/*" element={<RedirectFromUnregistered />} />
    </Routes>
  );
}

export default AgentRegistryRoutes;

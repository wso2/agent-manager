/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { generatePath, Navigate, Route, Routes, useParams } from "react-router-dom";
import { ProjectMCPProxyTable } from "./subComponents/ProjectMCPProxyTable";
import { ViewProjectMCPProxy } from "./subComponents/ViewProjectMCPProxy";

/** The MCP servers a project can use: the org's and the ones published from its components. */
export const MCPProxiesProject = () => {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();

  return (
    <Routes>
      <Route
        index
        element={
          <PageLayout title="MCP Servers" disableIcon>
            <ProjectMCPProxyTable />
          </PageLayout>
        }
      />
      <Route path="view/:proxyId" element={<ViewProjectMCPProxy />} />
      <Route
        path="*"
        element={
          <Navigate
            to={generatePath(
              absoluteRouteMap.children.org.children.projects.children
                .mcpProxies.path,
              { orgId: orgId ?? "", projectId: projectId ?? "" },
            )}
            replace
          />
        }
      />
    </Routes>
  );
};

export default MCPProxiesProject;

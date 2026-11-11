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

import { useMemo } from "react";
import { generatePath, Navigate, useNavigate, useParams } from "react-router-dom";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import { AddProjectMCPProxyForm } from "./subComponents/AddProjectMCPProxyForm";

/** Registers a project MCP server by its URL, with the same form as an org server. */
export function AddMCPProxyProject() {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const navigate = useNavigate();
  const { canWrite } = useResourceAccess("mcp_server");
  const backHref = useMemo(
    () =>
      generatePath(
        absoluteRouteMap.children.org.children.projects.children.mcpProxies.path,
        { orgId: orgId ?? "", projectId: projectId ?? "" },
      ),
    [orgId, projectId],
  );

  if (!canWrite) {
    return <Navigate to={backHref} replace />;
  }

  return (
    <PageLayout
      title="Register MCP Server"
      backHref={backHref}
      backLabel="Back to MCP server list"
      disableIcon
    >
      <AddProjectMCPProxyForm
        projectId={projectId ?? ""}
        onCancel={() => navigate(backHref)}
      />
    </PageLayout>
  );
}

export default AddMCPProxyProject;

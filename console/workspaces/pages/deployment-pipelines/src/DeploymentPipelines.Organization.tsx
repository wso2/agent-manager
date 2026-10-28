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

import { type ReactNode, useState } from "react";
import {
  generatePath,
  Navigate,
  Route,
  Routes,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import { DeploymentPipelineTable } from "./subComponents/DeploymentPipelineTable";
import { CreateDeploymentPipelineDrawer } from "./subComponents/CreateDeploymentPipelineDrawer";
import { DeploymentPipelineEditPage } from "./DeploymentPipelineEditPage";

/** Editing used to open a drawer at `?edit=<name>`; forward those links to the page. */
function LegacyEditRedirect({ children }: { children: ReactNode }) {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const editParam = searchParams.get("edit");
  if (editParam && orgId) {
    return (
      <Navigate
        to={generatePath(
          absoluteRouteMap.children.org.children.deploymentPipelines.children
            .view.path,
          // generatePath only stringifies params, and this one arrives raw
          // from the query string.
          { orgId, pipelineName: encodeURIComponent(editParam) },
        )}
        replace
      />
    );
  }
  return <>{children}</>;
}

export function DeploymentPipelinesOrganization() {
  const { orgId } = useParams<{ orgId: string }>();
  const [createOpen, setCreateOpen] = useState(false);

  const listHref = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.deploymentPipelines.path,
        { orgId },
      )
    : "/";

  return (
    <>
      <Routes>
        <Route
          index
          element={
            <LegacyEditRedirect>
              <PageLayout title="Deployment Pipelines" disableIcon>
                <DeploymentPipelineTable
                  onCreatePipeline={() => setCreateOpen(true)}
                />
              </PageLayout>
            </LegacyEditRedirect>
          }
        />
        <Route path=":pipelineName" element={<DeploymentPipelineEditPage />} />
        <Route path="*" element={<Navigate to={listHref} replace />} />
      </Routes>

      {orgId && (
        <CreateDeploymentPipelineDrawer
          open={createOpen}
          onClose={() => setCreateOpen(false)}
          orgId={orgId}
        />
      )}
    </>
  );
}

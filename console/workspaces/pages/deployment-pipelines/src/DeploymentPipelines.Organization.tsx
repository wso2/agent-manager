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

import { useEffect, useState } from "react";
import { Navigate, Route, Routes, useParams, useSearchParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import type { DeploymentPipelineResponse } from "@agent-management-platform/types";
import { useListDeploymentPipelines } from "@agent-management-platform/api-client";
import { DeploymentPipelineTable } from "./subComponents/DeploymentPipelineTable";
import { EditDeploymentPipelineDrawer } from "./subComponents/EditDeploymentPipelineDrawer";
import { CreateDeploymentPipelineDrawer } from "./subComponents/CreateDeploymentPipelineDrawer";

export function DeploymentPipelinesOrganization() {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const [pipelineToEdit, setPipelineToEdit] = useState<DeploymentPipelineResponse | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  const { data: pipelinesData } = useListDeploymentPipelines({ orgName: orgId });

  // Auto-open the edit drawer when ?edit=<pipelineName> is present in the URL.
  useEffect(() => {
    const editParam = searchParams.get("edit");
    if (!editParam || !pipelinesData?.deploymentPipelines) return;
    const match = pipelinesData.deploymentPipelines.find((p) => p.name === editParam);
    if (match) {
      setPipelineToEdit(match);
      const next = new URLSearchParams(searchParams);
      next.delete("edit");
      setSearchParams(next, { replace: true });
    }
  }, [searchParams, pipelinesData, setSearchParams]);

  return (
    <>
      <Routes>
        <Route
          index
          element={
            <PageLayout docs={["deploymentPipeline", "environmentManagement", "environment"]} title="Deployment Pipelines" disableIcon>
              <DeploymentPipelineTable
                onEditPipeline={setPipelineToEdit}
                onCreatePipeline={() => setCreateOpen(true)}
              />
            </PageLayout>
          }
        />
        <Route path="*" element={<Navigate to={`/org/${orgId}/deployment-pipelines`} replace />} />
      </Routes>

      {orgId && (
        <CreateDeploymentPipelineDrawer
          open={createOpen}
          onClose={() => setCreateOpen(false)}
          orgId={orgId}
        />
      )}

      {pipelineToEdit && orgId && (
        <EditDeploymentPipelineDrawer
          open={pipelineToEdit !== null}
          onClose={() => setPipelineToEdit(null)}
          pipeline={pipelineToEdit}
          orgId={orgId}
        />
      )}
    </>
  );
}

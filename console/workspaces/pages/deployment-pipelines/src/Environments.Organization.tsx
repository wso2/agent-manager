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

import { useState, type ComponentType } from "react";
import { Navigate, Route, Routes, useParams } from "react-router-dom";
import {
  PageLayout,
  useExternalComponentModulesByMountPoint,
} from "@agent-management-platform/views";
import type { Environment } from "@agent-management-platform/types";
import { EnvironmentTable } from "./subComponents/EnvironmentTable";
import { EditEnvironmentDrawer } from "./subComponents/EditEnvironmentDrawer";
import { CreateEnvironmentDrawer } from "./subComponents/CreateEnvironmentDrawer";
import { DeleteEnvironmentDrawer } from "./subComponents/DeleteEnvironmentDrawer";
import { EnvironmentViewPage } from "./EnvironmentViewPage";
import { ViewThunderInstance } from "./subComponents/ViewThunderInstance";

const ENVIRONMENT_CREATE_DRAWER_MOUNT_POINT = "environment-create-drawer";
const ENVIRONMENT_DELETE_DRAWER_MOUNT_POINT = "environment-delete-drawer";

type CreateDrawerProps = {
  open: boolean;
  onClose: () => void;
  orgId: string;
};

type DeleteDrawerProps = {
  open: boolean;
  onClose: () => void;
  environment: Environment;
  orgId: string;
};

export function EnvironmentsOrganization() {
  const { orgId } = useParams<{ orgId: string }>();
  const [envToEdit, setEnvToEdit] = useState<Environment | null>(null);
  const [envToDelete, setEnvToDelete] = useState<Environment | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  // A deployment can override the create/delete drawers; otherwise fall back to
  // the built-in script-based drawers.
  const externalCreate = useExternalComponentModulesByMountPoint(
    ENVIRONMENT_CREATE_DRAWER_MOUNT_POINT,
  )[0];
  const externalDelete = useExternalComponentModulesByMountPoint(
    ENVIRONMENT_DELETE_DRAWER_MOUNT_POINT,
  )[0];

  const CreateDrawer = (externalCreate?.component ??
    CreateEnvironmentDrawer) as ComponentType<CreateDrawerProps>;
  const DeleteDrawer = (externalDelete?.component ??
    DeleteEnvironmentDrawer) as ComponentType<DeleteDrawerProps>;

  return (
    <>
      <Routes>
        <Route
          index
          element={
            <PageLayout docs={["environmentManagement", "environment", "deploymentPipeline"]} title="Environments" disableIcon>
              <EnvironmentTable
                onEditEnvironment={setEnvToEdit}
                onCreateEnvironment={() => setCreateOpen(true)}
                onDeleteEnvironment={setEnvToDelete}
              />
            </PageLayout>
          }
        />
        <Route path=":envName" element={<EnvironmentViewPage />} />
        <Route path=":envName/identity-provider" element={<ViewThunderInstance />} />
        <Route
          path="*"
          element={<Navigate to={`/org/${orgId}/environments`} replace />}
        />
      </Routes>

      {createOpen && orgId && (
        <CreateDrawer
          open={createOpen}
          onClose={() => setCreateOpen(false)}
          orgId={orgId}
        />
      )}

      {envToEdit && orgId && (
        <EditEnvironmentDrawer
          open
          onClose={() => setEnvToEdit(null)}
          environment={envToEdit}
          orgId={orgId}
        />
      )}

      {envToDelete && orgId && (
        <DeleteDrawer
          open
          onClose={() => setEnvToDelete(null)}
          environment={envToDelete}
          orgId={orgId}
        />
      )}
    </>
  );
}

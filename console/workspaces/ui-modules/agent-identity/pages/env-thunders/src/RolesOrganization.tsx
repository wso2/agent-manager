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
import { Link, Navigate, Route, Routes, generatePath, useParams } from "react-router-dom";
import { Button } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { RolesPage } from "./subComponents/agentIdentity/RolesPage";
import { RoleCreatePage } from "./subComponents/agentIdentity/RoleCreatePage";
import { RoleEditPage } from "./subComponents/agentIdentity/RoleEditPage";

const rolesNode = absoluteRouteMap.children.org.children.agentIdentity.children.roles;
const TITLE = "Roles";

function RolesListPage() {
  const { orgId } = useParams<{ orgId: string }>();
  const createPath = orgId
    ? generatePath(rolesNode.children.create.path, { orgId })
    : "#";
  const { canWrite } = useResourceAccess("agent_identity");

  // One definition, used above the list and as the empty state's CTA.
  const createButton = canWrite ? (
    <Button
      variant="contained"
      color="primary"
      component={Link}
      to={createPath}
      startIcon={<Plus size={16} />}
    >
      Create role
    </Button>
  ) : undefined;

  return (
    <PageLayout
      title={TITLE}
      description="Roles available to agent identities."
      disableIcon
    >
      <RolesPage createAction={createButton} />
    </PageLayout>
  );
}

function RoleCreateWrapper() {
  const { orgId } = useParams<{ orgId: string }>();
  const backHref = generatePath(rolesNode.path, { orgId: orgId ?? "" });
  const { canWrite } = useResourceAccess("agent_identity");
  if (!canWrite) {
    return <Navigate to={backHref} replace />;
  }
  return (
    <PageLayout title="Create Role" backHref={backHref} backLabel="Back to roles" disableIcon>
      <RoleCreatePage />
    </PageLayout>
  );
}

export const RolesOrganization: React.FC = () => {
  return (
    <Routes>
      <Route index element={<RolesListPage />} />
      <Route path="create" element={<RoleCreateWrapper />} />
      <Route path=":roleId" element={<RoleEditPage />} />
      <Route path="*" element={<Navigate to="." replace />} />
    </Routes>
  );
};

export default RolesOrganization;

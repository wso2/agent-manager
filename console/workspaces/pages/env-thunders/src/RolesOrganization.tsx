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
import { Navigate, Route, Routes, generatePath, useParams, useSearchParams } from "react-router-dom";
import { absoluteRouteMap, isIdentityPromotionUiEnabled } from "@agent-management-platform/types";
import { PromotedWithAgentNote } from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { RolesPage } from "./subComponents/agentIdentity/RolesPage";
import { RoleCreatePage } from "./subComponents/agentIdentity/RoleCreatePage";
import { RoleEditPage } from "./subComponents/agentIdentity/RoleEditPage";
import { AgentIdentityEnvironmentGate } from "./subComponents/agentIdentity/AgentIdentityEnvironmentGate";
import { withSearchParams } from "./utils/withSearchParams";

const rolesNode = absoluteRouteMap.children.org.children.thunderInstances.children.roles;
const TITLE = "Roles";

function RolesListPage() {
  return (
    <AgentIdentityEnvironmentGate title={TITLE} environmentAgnostic>
      {() => (
        <PageLayout
          title={TITLE}
          description={
            isIdentityPromotionUiEnabled()
              ? "Roles that agents in this organization can be given."
              : "Roles available in the selected environment's identity provider."
          }
          disableIcon
        >
          {isIdentityPromotionUiEnabled() && (
            <PromotedWithAgentNote variant="alert" />
          )}
          <RolesPage />
        </PageLayout>
      )}
    </AgentIdentityEnvironmentGate>
  );
}

function RoleCreateWrapper() {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const backHref = withSearchParams(generatePath(rolesNode.path, { orgId: orgId ?? "" }), searchParams);
  return (
    <AgentIdentityEnvironmentGate title="Create Role" environmentAgnostic>
      {() => (
        <PageLayout title="Create Role" backHref={backHref} backLabel="Back to Roles" disableIcon>
          {isIdentityPromotionUiEnabled() && (
            <PromotedWithAgentNote variant="alert" />
          )}
          <RoleCreatePage />
        </PageLayout>
      )}
    </AgentIdentityEnvironmentGate>
  );
}

function RoleEditWrapper() {
  return (
    <AgentIdentityEnvironmentGate title="Role" environmentAgnostic>
      {() => <RoleEditPage />}
    </AgentIdentityEnvironmentGate>
  );
}

export const RolesOrganization: React.FC = () => {
  return (
    <Routes>
      <Route index element={<RolesListPage />} />
      <Route path="create" element={<RoleCreateWrapper />} />
      <Route path=":roleId" element={<RoleEditWrapper />} />
      <Route path="*" element={<Navigate to="." replace />} />
    </Routes>
  );
};

export default RolesOrganization;

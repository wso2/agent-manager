/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import {
  Navigate,
  Route,
  Routes,
  useParams,
  generatePath,
} from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { UsersPage } from "./UsersPage";
import { UserInvitePage } from "./UserInvitePage";
import { UserAddPage } from "./UserAddPage";
import { UserEditPage } from "./UserEditPage";
import { RolesPage } from "./RolesPage";
import { RoleCreatePage } from "./RoleCreatePage";
import { RoleEditPage } from "./RoleEditPage";
import { GroupsPage } from "./GroupsPage";
import { GroupCreatePage } from "./GroupCreatePage";
import { GroupEditPage } from "./GroupEditPage";

export const IdentitiesOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const { canWrite: canWriteMembers } = useResourceAccess("member");
  const { canWrite: canWriteRoles } = useResourceAccess("role");
  const { canWrite: canWriteGroups } = useResourceAccess("group");
  const identitiesRoute =
    absoluteRouteMap.children.org.children.settings.children.identities;
  // Gated create routes redirect; otherwise ":id" would match "create".
  const toList = (list: "users" | "roles" | "groups") => (
    <Navigate to={generatePath(identitiesRoute.children[list].path, { orgId })} replace />
  );

  return (
    <Routes>
      <Route
        index
        element={
          <Navigate
            to={generatePath(identitiesRoute.children.users.path, { orgId })}
            replace
          />
        }
      />
      <Route
        path="users/invite"
        element={canWriteMembers ? <UserInvitePage /> : toList("users")}
      />
      <Route
        path="users/add"
        element={canWriteMembers ? <UserAddPage /> : toList("users")}
      />
      <Route path="users/:userId" element={<UserEditPage />} />
      <Route path="users/*" element={<UsersPage />} />
      <Route
        path="roles/create"
        element={canWriteRoles ? <RoleCreatePage /> : toList("roles")}
      />
      <Route path="roles/:roleId" element={<RoleEditPage />} />
      <Route path="roles/*" element={<RolesPage />} />
      <Route
        path="groups/create"
        element={canWriteGroups ? <GroupCreatePage /> : toList("groups")}
      />
      <Route path="groups/:groupId" element={<GroupEditPage />} />
      <Route path="groups/*" element={<GroupsPage />} />
      <Route
        path="*"
        element={
          <Navigate
            to={generatePath(identitiesRoute.children.users.path, { orgId })}
            replace
          />
        }
      />
    </Routes>
  );
};

export default IdentitiesOrganization;

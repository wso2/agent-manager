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

import React, { lazy, Suspense } from "react";
import {
  generatePath,
  Navigate,
  Route,
  Routes,
  useParams,
} from "react-router-dom";
import { Folder, Shield, Users } from "@wso2/oxygen-ui-icons-react";
import { Skeleton, Stack } from "@wso2/oxygen-ui";
import {
  SettingsLayout,
  type SubNavItem,
  type SubNavSection,
} from "./SettingsLayout";
import { settingsRoute, useIdentityVisibility } from "./settingsRoutes";

// Only the org scope pulls in the identity pages, so it loads them on demand:
// a static import would put the whole tree in the project and agent chunks too.
const IdentitiesOrganization = lazy(() =>
  import("@agent-management-platform/identities").then((m) => ({
    default: m.IdentitiesOrganization,
  })),
);

/**
 * The organization rail — Users, Groups and Roles, each shown only when the
 * caller holds the identity scopes that page needs.
 */
function useOrganizationSections(): SubNavSection[] {
  const { orgId } = useParams<{ orgId: string }>();
  const visibility = useIdentityVisibility();
  const identityNode = settingsRoute.children.identities.children;

  const items: SubNavItem[] = [];
  if (visibility.users) {
    items.push({
      label: "Users",
      href: generatePath(identityNode.users.path, { orgId }),
      wildPath: identityNode.users.wildPath,
      icon: <Users size={18} />,
    });
  }
  if (visibility.groups) {
    items.push({
      label: "Groups",
      href: generatePath(identityNode.groups.path, { orgId }),
      wildPath: identityNode.groups.wildPath,
      icon: <Folder size={18} />,
    });
  }
  if (visibility.roles) {
    items.push({
      label: "Roles",
      href: generatePath(identityNode.roles.path, { orgId }),
      wildPath: identityNode.roles.wildPath,
      icon: <Shield size={18} />,
    });
  }

  return items.length > 0 ? [{ title: "User Management", items }] : [];
}

/** Organization-scope Settings: hosts the identity pages under the shared rail. */
export const SettingsOrganization: React.FC = () => {
  const sections = useOrganizationSections();
  return (
    <SettingsLayout sections={sections}>
      <Suspense
        fallback={
          <Stack spacing={2}>
            <Skeleton variant="rounded" height={56} />
            <Skeleton variant="rounded" height={160} />
          </Stack>
        }
      >
        <Routes>
          <Route index element={<Navigate to="identities/users" replace />} />
          <Route path="identities/*" element={<IdentitiesOrganization />} />
          <Route
            path="*"
            element={<Navigate to="identities/users" replace />}
          />
        </Routes>
      </Suspense>
    </SettingsLayout>
  );
};

export default SettingsOrganization;

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
import { GroupsPage } from "./subComponents/agentIdentity/GroupsPage";
import { GroupCreatePage } from "./subComponents/agentIdentity/GroupCreatePage";
import { GroupEditPage } from "./subComponents/agentIdentity/GroupEditPage";

const groupsNode = absoluteRouteMap.children.org.children.agentIdentity.children.groups;
const TITLE = "Groups";

function GroupsListPage() {
  const { orgId } = useParams<{ orgId: string }>();
  const createPath = orgId
    ? generatePath(groupsNode.children.create.path, { orgId })
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
      Create group
    </Button>
  ) : undefined;

  return (
    <PageLayout
      title={TITLE}
      description="Groups available to agent identities."
      disableIcon
    >
      <GroupsPage createAction={createButton} />
    </PageLayout>
  );
}

function GroupCreateWrapper() {
  const { orgId } = useParams<{ orgId: string }>();
  const backHref = generatePath(groupsNode.path, { orgId: orgId ?? "" });
  const { canWrite } = useResourceAccess("agent_identity");
  if (!canWrite) {
    return <Navigate to={backHref} replace />;
  }
  return (
    <PageLayout title="Create Group" backHref={backHref} backLabel="Back to groups" disableIcon>
      <GroupCreatePage />
    </PageLayout>
  );
}

export const GroupsOrganization: React.FC = () => {
  return (
    <Routes>
      <Route index element={<GroupsListPage />} />
      <Route path="create" element={<GroupCreateWrapper />} />
      <Route path=":groupId" element={<GroupEditPage />} />
      <Route path="*" element={<Navigate to="." replace />} />
    </Routes>
  );
};

export default GroupsOrganization;

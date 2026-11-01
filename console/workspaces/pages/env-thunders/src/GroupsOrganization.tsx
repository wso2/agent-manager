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
import { GroupsPage } from "./subComponents/agentIdentity/GroupsPage";
import { GroupCreatePage } from "./subComponents/agentIdentity/GroupCreatePage";
import { GroupEditPage } from "./subComponents/agentIdentity/GroupEditPage";
import { AgentIdentityEnvironmentGate } from "./subComponents/agentIdentity/AgentIdentityEnvironmentGate";
import { withSearchParams } from "./utils/withSearchParams";

const groupsNode = absoluteRouteMap.children.org.children.thunderInstances.children.groups;
const TITLE = "Groups";

function GroupsListPage() {
  return (
    <AgentIdentityEnvironmentGate title={TITLE} environmentAgnostic>
      {() => (
        <PageLayout
          title={TITLE}
          description={
            isIdentityPromotionUiEnabled()
              ? "Groups that agents in this organization can be given."
              : "Groups available in the selected environment's identity provider."
          }
          disableIcon
        >
          {isIdentityPromotionUiEnabled() && (
            <PromotedWithAgentNote variant="alert" />
          )}
          <GroupsPage />
        </PageLayout>
      )}
    </AgentIdentityEnvironmentGate>
  );
}

function GroupCreateWrapper() {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const backHref = withSearchParams(generatePath(groupsNode.path, { orgId: orgId ?? "" }), searchParams);
  return (
    <AgentIdentityEnvironmentGate title="Create Group" environmentAgnostic>
      {() => (
        <PageLayout title="Create Group" backHref={backHref} backLabel="Back to Groups" disableIcon>
          {isIdentityPromotionUiEnabled() && (
            <PromotedWithAgentNote variant="alert" />
          )}
          <GroupCreatePage />
        </PageLayout>
      )}
    </AgentIdentityEnvironmentGate>
  );
}

function GroupEditWrapper() {
  return (
    <AgentIdentityEnvironmentGate title="Group" environmentAgnostic>
      {() => <GroupEditPage />}
    </AgentIdentityEnvironmentGate>
  );
}

export const GroupsOrganization: React.FC = () => {
  return (
    <Routes>
      <Route index element={<GroupsListPage />} />
      <Route path="create" element={<GroupCreateWrapper />} />
      <Route path=":groupId" element={<GroupEditWrapper />} />
      <Route path="*" element={<Navigate to="." replace />} />
    </Routes>
  );
};

export default GroupsOrganization;

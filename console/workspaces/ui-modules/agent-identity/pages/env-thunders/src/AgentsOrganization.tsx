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
import { Navigate, Route, Routes } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { AgentsTab } from "./subComponents/agentIdentity/AgentsTab";
import { AgentDetailPage } from "./subComponents/agentIdentity/AgentDetailPage";
import { AgentIdentityEnvironmentSelector } from "./subComponents/agentIdentity/AgentIdentityEnvironmentSelector";
import { AgentIdentityEnvironmentGate } from "./subComponents/agentIdentity/AgentIdentityEnvironmentGate";

const TITLE = "Agents";

function AgentsListPage() {
  return (
    <AgentIdentityEnvironmentGate title={TITLE}>
      {() => (
        <PageLayout
          title={TITLE}
          description="Agents provisioned in the selected environment's identity provider."
          disableIcon
          actions={<AgentIdentityEnvironmentSelector />}
        >
          <AgentsTab />
        </PageLayout>
      )}
    </AgentIdentityEnvironmentGate>
  );
}

function AgentDetailWrapper() {
  return (
    <AgentIdentityEnvironmentGate title={TITLE}>
      {() => <AgentDetailPage />}
    </AgentIdentityEnvironmentGate>
  );
}

export const AgentsOrganization: React.FC = () => {
  return (
    <Routes>
      <Route index element={<AgentsListPage />} />
      <Route path=":projectName/:agentName" element={<AgentDetailWrapper />} />
      <Route path="*" element={<Navigate to="." replace />} />
    </Routes>
  );
};

export default AgentsOrganization;

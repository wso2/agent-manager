/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { Navigate, Route, Routes, generatePath, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useFeatureAccess } from "@agent-management-platform/feature-management";
import { ExternalAgentFlow } from "./components/ExternalAgentFlow";

/** Create-agent wizard: registers an externally-hosted agent. */
export const AddNewAgent: React.FC = () => {
  const canCreate = useFeatureAccess("agent_create");
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId?: string;
  }>();

  if (!canCreate) {
    return (
      <Navigate
        to={generatePath(absoluteRouteMap.children.org.children.projects.path, {
          orgId: orgId ?? '',
          projectId: projectId ?? 'default',
        })}
        replace
      />
    );
  }

  return (
    <Routes>
      <Route index element={<ExternalAgentFlow />} />
      <Route path="*" element={<Navigate to="." replace />} />
    </Routes>
  );
};

export default AddNewAgent;

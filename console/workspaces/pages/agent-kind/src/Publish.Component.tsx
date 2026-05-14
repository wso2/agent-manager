/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
import { PublishedList } from "./Publish.List";
import { PublishVersionDetails } from "./Publish.VersionDetails";

export const PublishComponent: React.FC = () => {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  return (
    <Routes>
      <Route index element={<PublishedList />} />
      <Route path="create-new-version" element={<PublishedList />} />
      <Route path="version-details/:versionId" element={<PublishVersionDetails />} />
      <Route path="version-details/:versionId/edit" element={<PublishVersionDetails />} />
      <Route
        path="*"
        element={
          <Navigate
            to={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents.children.publish.path,
              { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
            )}
          />
        }
      />
    </Routes>
  );
};

export default PublishComponent;

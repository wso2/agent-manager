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

import React, { useCallback } from "react";
import { Route, Routes, generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { NewAgentOptions } from "./components/NewAgentOptions";
import { InternalAgentFlow } from "./components/InternalAgentFlow";
import { ExternalAgentFlow } from "./components/ExternalAgentFlow";

export const AddNewAgent: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId?: string;
  }>();

  const NEW_AGENT_ROUTES = absoluteRouteMap.children.org.children.projects.children.newAgent;
  const CREATE_PATTERN = NEW_AGENT_ROUTES.children.create.path;
  const CONNECT_PATTERN = NEW_AGENT_ROUTES.children.connect.path;

  const handleSelect = useCallback((option: 'new' | 'existing') => {
    const target = option === 'new' ? CREATE_PATTERN : CONNECT_PATTERN;
    navigate(generatePath(target, {
      orgId: orgId ?? '',
      projectId: projectId ?? 'default',
    }));
  }, [navigate, orgId, projectId, CREATE_PATTERN, CONNECT_PATTERN]);

  return (
    <Routes>
      <Route index element={<NewAgentOptions onSelect={handleSelect} />} />
      <Route path="create" element={<InternalAgentFlow />} />
      <Route path="connect" element={<ExternalAgentFlow />} />
    </Routes>
  );
};

export default AddNewAgent;

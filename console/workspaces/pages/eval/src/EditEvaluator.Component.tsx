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
import { Navigate, generatePath, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

/**
 * Redirect legacy /edit routes to the unified view page.
 * Editing is now handled inline on the ViewEvaluator page.
 */
export const EditEvaluatorComponent: React.FC = () => {
  const { agentId, orgId, projectId, evaluatorId } = useParams<{
    agentId: string;
    orgId: string;
    projectId: string;
    evaluatorId: string;
  }>();

  const evaluatorsRouteMap = agentId
    ? absoluteRouteMap.children.org.children.projects.children.agents.children
        .evaluation.children.evaluators
    : absoluteRouteMap.children.org.children.projects.children.evaluators;

  const routeParams = agentId
    ? { orgId, projectId, agentId }
    : { orgId, projectId };

  const viewHref = generatePath(evaluatorsRouteMap.children.view.path, {
    ...routeParams,
    evaluatorId,
  });

  return <Navigate to={viewHref} replace />;
};

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
import { absoluteRouteMap, relativeRouteMap } from "@agent-management-platform/types";
import { NewAgentOptions } from "./components/NewAgentOptions";
import { NewAgentSourceOptions } from "./components/NewAgentSourceOptions";
import { InternalAgentFlow } from "./components/InternalAgentFlow";
import { CatalogAgentFlow } from "./components/CatalogAgentFlow";
import { CatalogKindSelect } from "./components/CatalogKindSelect";
import { ExternalAgentFlow } from "./components/ExternalAgentFlow";
import { ConsoleAction, useTrack } from "@agent-management-platform/api-client";

/**
 * Create-agent wizard.
 *
 * Steps are routes, so each one already produces a page view. What is tracked
 * here on top of that is the *choice* made at each fork — which the route alone
 * does not say, since a user can also arrive by deep link.
 *
 * Abandonment is deliberately not emitted from here. The wizard has no single
 * unmount that means "gave up" (each flow navigates away on success), so any
 * client-side flag would be bookkeeping that drifts. It is instead derived in
 * Moesif: a wizard-step-viewed with no amp.agent-development.create-agent event
 * later in the same session is an abandonment, and that join is exactly what
 * having both streams under one user_id is for.
 */
export const AddNewAgent: React.FC = () => {
  const navigate = useNavigate();
  const { track } = useTrack();
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId?: string;
  }>();

  const NEW_AGENT_ROUTES = absoluteRouteMap.children.org.children.projects.children.newAgent;
  const CREATE_PATTERN = NEW_AGENT_ROUTES.children.create.path;
  const CONNECT_PATTERN = NEW_AGENT_ROUTES.children.connect.path;

  const handleSelect = useCallback((option: 'new' | 'existing') => {
    track(ConsoleAction.WizardStepViewed, {
      wizard: "create-agent",
      step: option === 'new' ? "build-new" : "connect-existing",
      step_index: 1,
    });
    const target = option === 'new' ? CREATE_PATTERN : CONNECT_PATTERN;
    navigate(generatePath(target, {
      orgId: orgId ?? '',
      projectId: projectId ?? 'default',
    }));
  }, [navigate, orgId, projectId, CREATE_PATTERN, CONNECT_PATTERN, track]);

  const handleSourceSelect = useCallback((option: 'source' | 'catalog') => {
    track(ConsoleAction.WizardStepViewed, {
      wizard: "create-agent",
      step: option === 'source' ? "from-source" : "from-catalog",
      step_index: 2,
    });
    navigate(generatePath(`${CREATE_PATTERN}/${option}`, {
      orgId: orgId ?? '',
      projectId: projectId ?? 'default',
    }));
  }, [navigate, orgId, projectId, CREATE_PATTERN, track]);

  return (
    <Routes>
      <Route index element={<NewAgentOptions onSelect={handleSelect} />} />
        <Route
          path={
            relativeRouteMap.children.org.children.projects.children.newAgent
              .children.create.path
          }
        >
          <Route index element={<NewAgentSourceOptions onSelect={handleSourceSelect} />} />
          <Route
            path={
              relativeRouteMap.children.org.children.projects.children.newAgent
                .children.create.children.catalog.path
            }
          >
            <Route index element={<CatalogKindSelect />} />
            <Route
              path={
                relativeRouteMap.children.org.children.projects.children.newAgent
                  .children.create.children.catalog.children.withKind.path
              }
              element={<CatalogAgentFlow />}
            />
          </Route>
          <Route
            path={
              relativeRouteMap.children.org.children.projects.children.newAgent
                .children.create.children.source.path
            }
            element={<InternalAgentFlow />}
          />
      </Route>
      <Route path="connect" element={<ExternalAgentFlow />} />
    </Routes>
  );
};

export default AddNewAgent;

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
import { AgentChat } from "./AgentTest/AgentChat";
import { FadeIn, PageLayout } from "@agent-management-platform/views";
import {
  Box,
  Tab,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronsLeftRight, MessageCircle } from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  Link,
  Navigate,
  Route,
  Routes,
  useMatch,
  useParams,
} from "react-router-dom";
import {
  absoluteRouteMap,
  relativeRouteMap,
} from "@agent-management-platform/types";
import { Swagger } from "./AgentTest/Swagger";

export const TestComponent: React.FC = () => {
  const { orgId, projectId, agentId, envId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
    envId: string;
  }>();

  const isChatView = useMatch(
    absoluteRouteMap.children.org.children.projects.children.agents.children
      .environment.children.tryOut.children.chat.path
  );

  return (
    <FadeIn>
      <PageLayout
        title={"Try your agent"}
        disableIcon
        actions={
          <Tabs value={isChatView ? 0 : 1}>
            <Tab
              component={Link}
              to={generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.environment.children.tryOut.children.chat.path,
                { orgId, projectId, agentId, envId }
              )}
              label={
                <Box display="flex" alignItems="center" gap={1}>
                  <MessageCircle size={16} />
                  <Typography variant="body2">Chat</Typography>
                </Box>
              }
            />
            <Tab
              component={Link}
              to={generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.environment.children.tryOut.children.api.path,
                { orgId, projectId, agentId, envId }
              )}
              label={
                <Box display="flex" alignItems="center" gap={1}>
                  <ChevronsLeftRight size={16} />
                  <Typography variant="body2">API</Typography>
                </Box>
              }
            />
          </Tabs>
        }
      >
        <Routes>
          <Route
            path={
              relativeRouteMap.children.org.children.projects.children.agents
                .children.environment.children.tryOut.wildPath
            }
          >
            <Route
              path={
                relativeRouteMap.children.org.children.projects.children.agents
                  .children.environment.children.tryOut.children.chat.path
              }
              element={<AgentChat />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.projects.children.agents
                  .children.environment.children.tryOut.children.api.path
              }
              element={<Swagger/>}
            />
          </Route>
          <Route
            path="*"
            element={
              <Navigate
                to={generatePath(
                  absoluteRouteMap.children.org.children.projects.children
                    .agents.children.environment.children.tryOut.children.chat
                    .path,
                  { orgId, projectId, agentId, envId }
                )}
              />
            }
          />
        </Routes>
      </PageLayout>
    </FadeIn>
  );
};

export default TestComponent;

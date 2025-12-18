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

import React, { useState } from "react";
import { Box } from "@wso2/oxygen-ui";
import { TracesTable } from "@agent-management-platform/shared-component";
import { FadeIn, PageLayout } from "@agent-management-platform/views";
import { generatePath, Route, Routes, useParams } from "react-router-dom";
import { TraceDetails } from "./subComponents/TraceDetails";
import {
  absoluteRouteMap,
  relativeRouteMap,
  TraceListTimeRange,
} from "@agent-management-platform/types";
import { useGetAgent } from "@agent-management-platform/api-client";

export const AgentTraces: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams();
  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const [timeRange, setTimeRange] = useState<TraceListTimeRange>(
    TraceListTimeRange.ONE_DAY
  );

  return (
    <FadeIn>
      <Box
        sx={{
          display: "flex",
          pb: 2,
          flexDirection: "column",
        }}
      >
        <Routes>
          <Route
            path={
              agent?.provisioning.type === "internal"
                ? relativeRouteMap.children.org.children.projects.children
                    .agents.children.environment.children.observability.children
                    .traces.path + "/*"
                : relativeRouteMap.children.org.children.projects.children
                    .agents.children.observe.children.traces.path + "/*"
            }
          >
            <Route
              index
              element={
                <PageLayout title="Traces" disableIcon>
                  <TracesTable
                    orgId={orgId ?? "default"}
                    projectId={projectId ?? "default"}
                    agentId={agentId ?? "default"}
                    envId={envId ?? "default"}
                    timeRange={timeRange}
                    setTimeRange={setTimeRange}
                  />
                </PageLayout>
              }
            />
            <Route
              path={
                agent?.provisioning.type === "internal"
                  ? relativeRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.observability
                      .children.traces.children.traceDetails.path
                  : relativeRouteMap.children.org.children.projects.children
                      .agents.children.observe.children.traces.children
                      .traceDetails.path
              }
              element={
                <PageLayout
                  title="Trace Details"
                  backLabel="Back to Traces"
                  disableIcon
                  backHref={generatePath(
                    agent?.provisioning.type === "internal"
                      ? absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.observability.children
                      .traces.path
                      : absoluteRouteMap.children.org.children.projects.children
                      .agents.children.observe.children.traces.path,
                    { orgId, projectId, agentId, envId }
                  )}
                >
                  <TraceDetails />
                </PageLayout>
              }
            />
          </Route>
        </Routes>
      </Box>
    </FadeIn>
  );
};

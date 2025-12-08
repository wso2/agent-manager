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
    orgName: orgId ?? "",
    projName: projectId ?? "",
    agentName: agentId ?? "",
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

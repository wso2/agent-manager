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

import { Box, Typography, Button } from "@wso2/oxygen-ui";
import { Clock as AccessTime, Settings } from "@wso2/oxygen-ui-icons-react";
import { useParams, useSearchParams } from "react-router-dom";
import dayjs from "dayjs";
import { useGetAgent } from "@agent-management-platform/api-client";
import { EnvironmentCard } from "@agent-management-platform/shared-component";
import { InstrumentationDrawer } from "./InstrumentationDrawer";

export const ExternalAgentOverview = () => {
  const { agentId, orgId, projectId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const isInstrumentationDrawerOpen = searchParams.get("setup") === "true";

  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  // Sample instrumentation config - these would come from props or API
  const instrumentationUrl = "http://localhost:21893";
  const apiKey = "00000000-0000-0000-0000-000000000000";

  return (
    <>
      <Box display="flex" flexDirection="column" pb={4} gap={4}>
        <Box
          sx={{
            maxWidth: "fit-content",
            gap: 1.5,
            display: "flex",
            flexDirection: "column",
            width: "50%",
          }}
        >
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            <Typography variant="body2">Created</Typography>
            <AccessTime size={14} />
            <Typography variant="body2">
              {agent?.createdAt ? dayjs(agent.createdAt).fromNow() : '—'}
            </Typography>
          </Box>
        </Box>
        <EnvironmentCard
          external
          orgId={orgId ?? "default"}
          projectId={projectId ?? "default"}
          agentId={agentId ?? "default"}
          actions={
            <Button
              variant="text"
              size="small"
              startIcon={<Settings size={16} />}
              onClick={() => setSearchParams({ setup: "true" })}
            >
              Setup Agent
            </Button>
          }
        />
      </Box>
      <InstrumentationDrawer
        open={isInstrumentationDrawerOpen}
        onClose={() => setSearchParams({})}
        agentId={agentId ?? ""}
        instrumentationUrl={instrumentationUrl}
        apiKey={apiKey}
      />
    </>
  );
};

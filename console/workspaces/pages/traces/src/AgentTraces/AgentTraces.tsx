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

import React, { useMemo, useState } from 'react';
import { Box, Collapse, useTheme } from '@mui/material';
import { TopCards } from './subComponents/TopCards';
import { TracesTable } from './subComponents/TracesTable';
import { FadeIn } from '@agent-management-platform/views';
import { useMatch, useParams } from 'react-router-dom';
import { TraceDetails } from './subComponents/TraceDetails';
import { absoluteRouteMap, TraceListTimeRange } from '@agent-management-platform/types';
import { useGetAgent } from '@agent-management-platform/api-client';

export const AgentTraces: React.FC = () => {
  const { agentId, orgId, projectId } = useParams();
  const { data: agent } = useGetAgent({
    orgName: orgId ?? '',
    projName: projectId ?? '',
    agentName: agentId ?? '',
  });
  const theme = useTheme();
  const [timeRange, setTimeRange] = useState<TraceListTimeRange>(TraceListTimeRange.ONE_DAY);
  const absoluteRoutePattern = useMemo(() => {
    if (agent?.provisioning.type === "internal") {
      return absoluteRouteMap.children.org.
        children.projects.children.agents.children.environment.
        children.observability.children.traces.path
    }
    return absoluteRouteMap.children.org.children.projects.children.agents.children.traces.path;
  }, [agent]);
  const isTraceDetails = useMatch(absoluteRoutePattern);
  return (
    <FadeIn>
      <Box
        sx={{
          pt: theme.spacing(1),
          gap: theme.spacing(2),
          display: 'flex',
          flexDirection: 'column'
        }}
      >
        <Collapse in={!isTraceDetails}>
          <TopCards timeRange={timeRange} />
        </Collapse>
        {
          isTraceDetails ? (
            <TraceDetails />
          ) : (
            <TracesTable timeRange={timeRange} setTimeRange={setTimeRange} />
          )
        }
      </Box>
    </FadeIn>
  );
};


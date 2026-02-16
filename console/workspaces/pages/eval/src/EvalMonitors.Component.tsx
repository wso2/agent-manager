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
import { PageLayout } from "@agent-management-platform/views";
import { Button, Card, CardContent, LinearProgress, Stack, Typography } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";

const PLACEHOLDER_MONITORS = [
  {
    title: "Quality gates",
    copy: "Track eval thresholds that must stay green before deployments proceed.",
    progress: 70,
  },
  {
    title: "Live drifts",
    copy: "Surface conversational drift alerts that combine eval scores with runtime telemetry.",
    progress: 35,
  },
  {
    title: "Human review",
    copy: "Reserve runs that require qualitative grading and approvals before rollout.",
    progress: 55,
  },
];

export const EvalMonitorsComponent: React.FC = () => {
  const { agentId, envId, orgId, projectId } = useParams<{
    agentId: string, envId: string, orgId: string, projectId: string
  }>();

  return (
    <PageLayout
      title="Eval Monitors"
      description="Summaries for evaluation runs once the runtime APIs are wired up."
      disableIcon
      actions={<Button variant="contained" component={Link} to={
        generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.environment.children.evaluation.children.monitor.children.create.path,
          { orgId: orgId, projectId: projectId, agentId: agentId, envId: envId }
        )
      } endIcon={<Plus />} color="primary" >Add monitor</Button>}
    >
      <Stack spacing={2.5}>
        {PLACEHOLDER_MONITORS.map((monitor) => (
          <Card key={monitor.title} variant="outlined">
            <CardContent>
              <Typography variant="subtitle1">{monitor.title}</Typography>
              <Typography variant="body2" color="text.secondary" mt={0.5}>
                {monitor.copy}
              </Typography>
              <LinearProgress variant="determinate" value={monitor.progress} sx={{ mt: 2 }} />
            </CardContent>
          </Card>
        ))}
      </Stack>
    </PageLayout>
  )
};

export default EvalMonitorsComponent;

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

import { useListAgentDeployments } from "@agent-management-platform/api-client";
import { Environment } from "@agent-management-platform/types/dist/api/deployments";
import { NoDataFound, TextInput } from "@agent-management-platform/views";
import {
  Clock,
  FlaskConical,
  Rocket,
  Workflow,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Divider,
  Typography,
} from "@wso2/oxygen-ui";
import {
  EnvStatus,
  DeploymentStatus,
} from "@agent-management-platform/shared-component";
import dayjs from "dayjs";
import { absoluteRouteMap } from "@agent-management-platform/types";

interface DeployCardProps {
  currentEnvironment: Environment;
}

export function DeployCard(props: DeployCardProps) {
  const { currentEnvironment } = props;
  const { orgId, agentId, projectId } = useParams();

  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments({
      orgName: orgId ?? "",
      projName: projectId ?? "",
      agentName: agentId ?? "",
    });
  const currentDeployment = deployments?.[currentEnvironment.name];

  if (isDeploymentsLoading) {
    return (
      <Card
        variant="outlined"
        sx={{
          "& .MuiCardContent-root": {
            backgroundColor: "background.paper",
            gap: 2,
            display: "flex",
            height: "100%",
            width: 350,
            justifyContent: "center",
            alignItems: "center",
            flexDirection: "column",
          },
        }}
      >
        <CardContent>
          <CircularProgress />
        </CardContent>
      </Card>
    );
  }

  if (!currentDeployment || currentDeployment.status === "not-deployed") {
    return (
      <Card
        variant="outlined"
        sx={{
          "& .MuiCardContent-root": {
            backgroundColor: "background.paper",
            gap: 2,
            display: "flex",
            width: 350,
          },
          height: "fit-content",
        }}
      >
        <CardContent>
          <Box
            display="flex"
            flexGrow={1}
            pt={2}
            justifyContent="center"
            alignItems="center"
          >
            <NoDataFound
              message="No Deployment found"
              icon={<Rocket size={32} />}
              disableBackground
            />
          </Box>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card
      variant="outlined"
      sx={{
        "& .MuiCardContent-root": {
          backgroundColor: "background.paper",
          gap: 2,
          display: "flex",
          flexDirection: "column",
          width: 350,
          height: "100%",
        },
      }}
    >
      <CardContent>
        <Box display="flex" flexDirection="row" gap={1}>
          <Typography variant="h4">
            {currentEnvironment?.displayName}
          </Typography>
          <EnvStatus status={currentDeployment?.status as DeploymentStatus} />
        </Box>
        <Divider />
        <Box display="flex" flexDirection="row" gap={1} alignItems="center">
          <Typography variant="body2">Last Deployed</Typography>
          <Clock size={16} />
          <Typography variant="body2">
            {dayjs(currentDeployment?.lastDeployed).fromNow()}
          </Typography>
        </Box>
        {currentDeployment?.imageId && (
          <TextInput
            label="Build Image"
            value={currentDeployment?.imageId}
            copyable
            copyTooltipText="Copy Build Image"
            slotProps={{
              input: {
                readOnly: true,
              },
            }}
          />
        )}
        {currentDeployment?.endpoints.map((endpoint) => (
          <TextInput
            key={endpoint.url}
            label="URL"
            value={endpoint.url}
            copyable
            copyTooltipText="Copy URL"
            slotProps={{
              input: {
                readOnly: true,
              },
            }}
          />
        ))}

        <Button
          variant="outlined"
          component={Link}
          to={generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents
              .children.environment.children.tryOut.path,
            {
              orgId,
              projectId,
              agentId,
              envId: currentEnvironment?.name,
            }
          )}
          size="small"
          startIcon={<FlaskConical size={16} />}
        >
          Try It
        </Button>
        <Button
          variant="text"
          component={Link}
          to={generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents
              .children.environment.children.observability.children.traces.path,
            {
              orgId,
              projectId,
              agentId,
              envId: currentEnvironment?.name,
            }
          )}
          size="small"
          startIcon={<Workflow size={16} />}
        >
          View Traces
        </Button>
      </CardContent>
    </Card>
  );
}

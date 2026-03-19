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

import {
  useListAgentDeployments,
  useUpdateDeploymentState,
} from "@agent-management-platform/api-client";
import { Environment } from "@agent-management-platform/types/dist/api/deployments";
import { NoDataFound, TextInput } from "@agent-management-platform/views";
import {
  Clock,
  ExternalLink,
  FlaskConical,
  Rocket,
  Workflow,
  PlayCircle,
  PauseCircle,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  alpha,
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  Divider,
  IconButton,
  Stack,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  DeploymentStatus,
  EnvStatus,
} from "@agent-management-platform/shared-component";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { extractBuildIdFromImageId } from "../utils/extractBuildIdFromImageId";
import { formatDistanceToNow } from "date-fns";
import { useMemo } from "react";

function DeploymentStatusPanel({ status }: { status: DeploymentStatus }) {
  const theme = useTheme();
  const backgroundColor = useMemo(() => {
    if (status === DeploymentStatus.ACTIVE) {
      return alpha(theme.palette.success.light, 0.1);
    }
    if (status === DeploymentStatus.INACTIVE) {
      return theme.palette.grey[200];
    }
    if (status === DeploymentStatus.DEPLOYING) {
      return alpha(theme.palette.warning.light, 0.1);
    }
    if (status === DeploymentStatus.ERROR) {
      return alpha(theme.palette.error.light, 0.1);
    }
    if (status === DeploymentStatus.SUSPENDED) {
      return theme.palette.grey[200];
    }
    return theme.palette.grey[200];
  }, [status, theme]);
  return (
    <Box
      display="flex"
      gap={1}
      flexGrow={1}
      alignItems="center"
      justifyContent="space-between"
      sx={{
        backgroundColor: backgroundColor,
        fillOpacity: 0.1,
        padding: 1,
        borderRadius: 0.5,
      }}
    >
      <Typography variant="body2">Deployment Status:</Typography>
      <EnvStatus status={status} />
    </Box>
  );
}

interface DeployCardProps {
  currentEnvironment: Environment;
}

export function DeployCard(props: DeployCardProps) {
  const { currentEnvironment } = props;
  const { orgId, agentId, projectId } = useParams();

  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });
  const { mutate: updateDeploymentState, isPending: isUpdating } =
    useUpdateDeploymentState();
  const currentDeployment = deployments?.[currentEnvironment.name];
  const selectedBuildId = extractBuildIdFromImageId(currentDeployment?.imageId);
  const lastDeployedText = currentDeployment?.lastDeployed
    ? formatDistanceToNow(new Date(currentDeployment.lastDeployed), {
        addSuffix: true,
      })
    : "Unknown";

  const handleStop = () => {
    if (!currentEnvironment?.name || !orgId || !projectId || !agentId) return;
    updateDeploymentState({
      params: {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      },
      body: {
        environment: currentEnvironment.name,
        state: "Undeploy",
      },
    });
  };

  const handleRedeploy = () => {
    if (!currentEnvironment?.name || !orgId || !projectId || !agentId) return;
    updateDeploymentState({
      params: {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      },
      body: {
        environment: currentEnvironment.name,
        state: "Active",
      },
    });
  };

  if (isDeploymentsLoading) {
    return (
      <Card
        variant="outlined"
        sx={{
          height: "fit-content",
          width: 350,
          minWidth: 350,
        }}
      >
        <CardContent>
          <Box p={2} display="flex" justifyContent="center" alignItems="center">
            <CircularProgress />
          </Box>
        </CardContent>
      </Card>
    );
  }

  if (!currentDeployment || currentDeployment.status === "not-deployed") {
    return (
      <Card
        variant="outlined"
        sx={{
          height: "fit-content",
          width: 350,
          minWidth: 350,
        }}
      >
        <CardContent>
          <Stack gap={2} alignItems="center">
            <NoDataFound
              message="No Deployment found"
              icon={<Rocket size={32} />}
              disableBackground
            />
          </Stack>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card
      variant="outlined"
      sx={{
        height: "fit-content",
        width: 400,
        minWidth: 400,
      }}
    >
      <CardContent>
        <Stack gap={2}>
          <Stack
            direction="row"
            gap={1}
            alignItems="center"
            justifyContent="space-between"
          >
            <Stack direction="row" gap={1} alignItems="center">
              <Typography variant="h5">
                {currentEnvironment?.displayName} Environment
              </Typography>
            </Stack>
            <Stack direction="row" height={15} gap={1} alignItems="center">
              {currentDeployment?.status !== DeploymentStatus.SUSPENDED && (
                <Button
                  startIcon={<PauseCircle size={16} />}
                  variant="outlined"
                  size="small"
                  onClick={handleStop}
                  disabled={
                    isUpdating ||
                    currentDeployment?.status !== DeploymentStatus.ACTIVE
                  }
                >
                  Suspend
                </Button>
              )}
              {currentDeployment?.status === DeploymentStatus.SUSPENDED && (
                <Button
                  startIcon={
                    isUpdating ? (
                      <CircularProgress size={14} />
                    ) : (
                      <PlayCircle size={16} />
                    )
                  }
                  variant="outlined"
                  color="success"
                  size="small"
                  onClick={handleRedeploy}
                  disabled={isUpdating}
                >
                  Re-deploy
                </Button>
              )}
            </Stack>
          </Stack>
          <Divider />
          <Stack direction="row" gap={1} alignItems="center">
            <Typography variant="body2">Last Deployed</Typography>
            <Clock size={16} />
            <Typography variant="body2">{lastDeployedText}</Typography>
          </Stack>
          <Stack direction="row" gap={1} alignItems="center">
            <DeploymentStatusPanel
              status={currentDeployment?.status as DeploymentStatus}
            />
          </Stack>
          {currentDeployment?.imageId && (
            <TextInput
              label="Build Image"
              labelAction={
                <IconButton
                  component={Link}
                  to={
                    generatePath(
                      absoluteRouteMap.children.org.children.projects.children
                        .agents.children.build.path,
                      {
                        orgId,
                        projectId,
                        agentId,
                      },
                    ) +
                    "?panel=logs&selectedBuild=" +
                    selectedBuildId
                  }
                >
                  <ExternalLink size={16} />
                </IconButton>
              }
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
          <Divider />
          <Divider />
          <Stack direction="row" justifyContent="center" spacing={2}>
            <Button
              variant="text"
              component={Link}
              to={generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.environment.children.tryOut.path,
                {
                  orgId,
                  projectId,
                  agentId,
                  envId: currentEnvironment?.name,
                },
              )}
              size="small"
              startIcon={<FlaskConical size={16} />}
            >
              Try It
            </Button>
            <Divider orientation="vertical"/>
            <Button
              variant="text"
              component={Link}
              to={generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.environment.children.observability.children.traces
                  .path,
                {
                  orgId,
                  projectId,
                  agentId,
                  envId: currentEnvironment?.name,
                },
              )}
              size="small"
              startIcon={<Workflow size={16} />}
            >
              View Traces
            </Button>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}

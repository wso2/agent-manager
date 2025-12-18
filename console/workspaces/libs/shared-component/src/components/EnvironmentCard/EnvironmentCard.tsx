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
  useGetAgent,
  useListAgentDeployments,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  Environment,
} from "@agent-management-platform/types";
import {
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Skeleton,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  CheckCircle as CheckCircleRounded,
  Circle as CircleOutlined,
  Clock,
  XCircle as ErrorOutlineRounded,
  Rocket as RocketLaunchOutlined,
  FlaskConical as TryOutlined,
  Workflow,
  Link as LinkOutlined,
} from "@wso2/oxygen-ui-icons-react";
import { NoDataFound, TextInput } from "@agent-management-platform/views";
import dayjs from "dayjs";
import { generatePath, Link } from "react-router-dom";

export enum DeploymentStatus {
  ACTIVE = "active",
  INACTIVE = "not-deployed",
  DEPLOYING = "in-progress",
  ERROR = "error",
}

export interface EnvironmentCardProps {
  environment?: Environment;
  orgId: string;
  projectId: string;
  agentId: string;
  external?: true;
  actions?: React.ReactNode;
}

export const EnvStatus = ({ status }: { status?: DeploymentStatus }) => {
  const theme = useTheme();
  if (!status) {
    return null;
  }
  if (status === DeploymentStatus.ACTIVE) {
    return (
      <Chip
        icon={
          <CheckCircleRounded size={16} color={theme.palette.success.main} />
        }
        variant="outlined"
        size="small"
        label="Deployed"
        color="success"
      />
    );
  }
  if (status === DeploymentStatus.INACTIVE) {
    return (
      <Chip
        icon={<CircleOutlined size={16} color={theme.palette.text.disabled} />}
        variant="outlined"
        size="small"
        label="Not Deployed"
        color="default"
      />
    );
  }
  if (status === DeploymentStatus.DEPLOYING) {
    return (
      <Chip
        icon={<CircularProgress size={16} color="warning" />}
        variant="outlined"
        size="small"
        label="Deploying"
        color="warning"
      />
    );
  }
  if (status === DeploymentStatus.ERROR) {
    return <Chip variant="outlined" size="small" label="Error" color="error" />;
  }
};

export const EnvironmentCard = (props: EnvironmentCardProps) => {
  const { environment, external, orgId, projectId, agentId, actions } = props;
  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments(
      {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      },
      {
        enabled: !!orgId && !!projectId && !!agentId && !external,
      }
    );
  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const currentDiployment = deployments?.[environment?.name ?? "default"];
  const theme = useTheme();
  if (isDeploymentsLoading) {
    return <Skeleton variant="rounded" height={100} />;
  }
  if (!currentDiployment) {
    return (
      <Card
        variant="outlined"
        sx={{
          "&.MuiCard-root": {
            backgroundColor: "background.paper",
          },
        }}
      >
        <CardContent>
          <Box
            display="flex"
            flexDirection="row"
            gap={1}
            justifyContent="space-between"
            alignItems="center"
          >
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              <Typography variant="h6">Default Environment</Typography>
              <Chip
                icon={
                  <LinkOutlined size={16} color={theme.palette.success.main} />
                }
                variant="outlined"
                size="small"
                label="Registered"
                color="success"
              />
              <Box
                display="flex"
                flexDirection="row"
                gap={1}
                alignItems="center"
              >
                <Clock size={16} color={theme.palette.text.secondary} />
                {agent?.createdAt ? dayjs(agent.createdAt).fromNow() : '—'}
              </Box>
            </Box>
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              {actions}
              <Button
                startIcon={<Workflow size={16} />}
                variant="text"
                component={Link}
                to={generatePath(
                  absoluteRouteMap.children.org.children.projects.children
                    .agents.children.observe.children.traces.path,
                  {
                    orgId,
                    projectId,
                    agentId,
                  }
                )}
                color="primary"
                size="small"
              >
                View Traces
              </Button>
            </Box>
          </Box>
        </CardContent>
      </Card>
    );
  }
  return (
    <Card
      variant="outlined"
      sx={{
        "&.MuiCard-root": {
          backgroundColor: "background.paper",
        },
      }}
    >
      <CardContent>
        <Box
          display="flex"
          flexDirection="row"
          gap={1}
          pb={1}
          justifyContent="space-between"
          alignItems="center"
        >
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            <Typography variant="h6">
              {environment?.displayName} Environment
            </Typography>
            <EnvStatus status={currentDiployment?.status as DeploymentStatus} />
            {currentDiployment?.status === DeploymentStatus.ACTIVE && (
              <Box
                display="flex"
                flexDirection="row"
                gap={1}
                alignItems="center"
              >
                <Clock size={16} color={theme.palette.text.secondary} />
                {dayjs(currentDiployment?.lastDeployed).fromNow()}
              </Box>
            )}
          </Box>
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            {currentDiployment?.status === DeploymentStatus.ACTIVE && (
              <>
                <Button
                  startIcon={<TryOutlined size={16} />}
                  variant="text"
                  // disabled
                  component={Link}
                  to={generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.tryOut.path,
                    {
                      orgId,
                      projectId,
                      agentId,
                      envId: environment?.name ?? "",
                    }
                  )}
                  color="primary"
                  size="small"
                >
                  Try It
                </Button>
                <Button
                  startIcon={<Workflow size={16} />}
                  variant="text"
                  component={Link}
                  to={generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.observability
                      .children.traces.path,
                    {
                      orgId,
                      projectId,
                      agentId,
                      envId: environment?.name ?? "",
                    }
                  )}
                  color="primary"
                  size="small"
                >
                  View Traces
                </Button>
                {actions}
              </>
            )}
          </Box>
        </Box>
        <Divider />
        <Box
          display="flex"
          width="100%"
          justifyContent="center"
          flexDirection="column"
          gap={1}
          pt={2}
          alignItems="center"
        >
          {currentDiployment.status === DeploymentStatus.INACTIVE && (
            <NoDataFound
              disableBackground
              message="Not Deployed"
              icon={<RocketLaunchOutlined size={32} />}
            />
          )}
          {currentDiployment.status === DeploymentStatus.DEPLOYING && (
            <NoDataFound
              disableBackground
              message="Deploying..."
              icon={<CircularProgress size={32} />}
            />
          )}
          {currentDiployment.status === DeploymentStatus.ERROR && (
            <NoDataFound
              disableBackground
              message="Deployment Failed"
              icon={
                <ErrorOutlineRounded
                  color={theme.palette.error.main}
                  size={32}
                />
              }
            />
          )}
          {currentDiployment.status === DeploymentStatus.ACTIVE && (
            <Box
              display="flex"
              flexGrow={1}
              flexDirection="column"
              width="100%"
              gap={4}
              alignItems="flex-start"
            >
              {currentDiployment?.endpoints?.map((endpoint) => (
                <TextInput
                  slotProps={{
                    input: {
                      readOnly: true,
                    },
                  }}
                  key={endpoint.url}
                  label="URL"
                  value={endpoint.url}
                  fullWidth
                />
              ))}
            </Box>
          )}
        </Box>
      </CardContent>
    </Card>
  );
};

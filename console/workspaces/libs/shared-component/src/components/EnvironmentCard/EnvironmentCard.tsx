import { useListAgentDeployments } from "@agent-management-platform/api-client";
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
import { TabStatus } from "../LinkTab";
import {
  CheckCircle as CheckCircleRounded,
  Circle as CircleOutlined,
  Clock,
  XCircle as ErrorOutlineRounded,
  Rocket as RocketLaunchOutlined,
  FlaskConical as TryOutlined,
  Workflow,
} from "@wso2/oxygen-ui-icons-react";
import { NoDataFound, TextInput } from "@agent-management-platform/views";
import dayjs from "dayjs";
import { generatePath, Link } from "react-router-dom";

export interface EnvironmentCardProps {
  environment?: Environment;
  orgId: string;
  projectId: string;
  agentId: string;
  external?: true;
  actions?: React.ReactNode;
}

export const EnvStatus = ({ status }: { status?: TabStatus }) => {
  const theme = useTheme();
  if (!status) {
    return null;
  }
  if (status === TabStatus.ACTIVE) {
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
  if (status === TabStatus.INACTIVE) {
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
  if (status === TabStatus.DEPLOYING) {
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
  if (status === TabStatus.ERROR) {
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
            <Typography variant="h6">Default Environment</Typography>
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
            <Typography variant="h6">{environment?.displayName}</Typography>
            <EnvStatus status={currentDiployment?.status as TabStatus} />
            {currentDiployment?.status === TabStatus.ACTIVE && (
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
            {currentDiployment?.status === TabStatus.ACTIVE && (
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
                  Try Out
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
          {currentDiployment.status === TabStatus.INACTIVE && (
            <NoDataFound
              message="Not Deployed"
              icon={<RocketLaunchOutlined size={32}  />}
            />
          )}
          {currentDiployment.status === TabStatus.DEPLOYING && (
            <NoDataFound
              message="Deploying..."
              icon={<CircularProgress size={32} />}
            />
          )}
          {currentDiployment.status === TabStatus.ERROR && (
            <NoDataFound
              message="Deployment Failed"
              icon={<ErrorOutlineRounded color={theme.palette.error.main} size={32}  />}
            />
          )}
          {currentDiployment.status === TabStatus.ACTIVE && (
            <Box
              display="flex"
              flexGrow={1}
              flexDirection="column"
              width="100%"
              gap={4}
              alignItems="flex-start"
            >
                {currentDiployment?.endpoints.map((endpoint) => (
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

import {
  useGetAgent,
  useGetAgentBuilds,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import {
  Clock as AccessTime,
  GitHub,
  CheckCircle,
} from "@wso2/oxygen-ui-icons-react";
import {
  Box,
  Button,
  CircularProgress,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { generatePath, Link, useParams } from "react-router-dom";
import { useMemo } from "react";
import dayjs from "dayjs";
import { EnvironmentCard } from "@agent-management-platform/shared-component";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const InternalAgentOverview = () => {
  const { orgId, agentId, projectId } = useParams();
  const { data: agent } = useGetAgent({
    orgName: orgId ?? "default",
    projName: projectId ?? "default",
    agentName: agentId ?? "",
  });
  const { data: buildList } = useGetAgentBuilds({
    orgName: orgId ?? "",
    projName: projectId ?? "",
    agentName: agentId ?? "",
  });
  const { data: environmentList } = useListEnvironments({
    orgName: orgId ?? "",
  });
  const theme = useTheme();

  const sortedEnvironmentList = useMemo(() => {
    return environmentList?.sort((_a, b) => {
      if (b.isProduction) {
        return -1;
      }
      return 0;
    });
  }, [environmentList]);

  const repositoryUrl = useMemo(
    () =>
      `${agent?.provisioning?.repository?.url}/tree/${agent?.provisioning?.repository?.branch}/${agent?.provisioning?.repository?.appPath ?? ""}`,
    [
      agent?.provisioning?.repository?.url,
      agent?.provisioning?.repository?.branch,
      agent?.provisioning?.repository?.appPath,
    ]
  );

  const loadingBuilds = useMemo(() => {
    return buildList?.builds.filter(
      (build) =>
        build.status === "BuildInProgress" || build.status === "BuildTriggered"
    );
  }, [buildList]);

  return (
    <Box display="flex" flexDirection="column" gap={1.5} pb={4}>
      <Box
        sx={{
          maxWidth: "fit-content",
          gap: 0.5,
          display: "flex",
          flexDirection: "column",
          width: "50%",
        }}
      >
        <Box display="flex" flexDirection="row" gap={1} alignItems="center">
          <Typography variant="body2">Created</Typography>
          <AccessTime size={16} />
          <Typography variant="body2">
            {dayjs(agent?.createdAt).fromNow()}
          </Typography>
        </Box>

        <Box display="flex" flexDirection="row" gap={1} alignItems="center">
          <Typography variant="body2" >
            Source Code:
          </Typography>
          <Button
            component="a"
            startIcon={
              <GitHub size={16} color={theme.palette.text.secondary} />
            }
            variant="text"
            color="inherit"
            size="small"
            href={repositoryUrl}
            target="_blank"
          >
            {repositoryUrl}
          </Button>
        </Box>
        <Box display="flex" flexDirection="row" gap={1} alignItems="center">
          <Typography variant="body2">
            Build Status:
          </Typography>
          {loadingBuilds?.length && loadingBuilds.length > 0 ? (
            <Button
              variant="text"
              size="small"
              color="inherit"
              component={Link}
              to={generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.build.path,
                {
                  orgId,
                  projectId,
                  agentId,
                }
              )}
              startIcon={<CircularProgress size={14} color="inherit" />}
            >
              Build In Progress
            </Button>
          ) : (
            <Button
              variant="text"
              size="small"
              color="inherit"
              component={Link}
              to={generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.build.path,
                {
                  orgId,
                  projectId,
                  agentId,
                }
              )}
              startIcon={<CheckCircle size={16} />}
            >
              Build Completed
            </Button>
          )}
        </Box>
      </Box>
      {sortedEnvironmentList?.length && (
        <>
          {sortedEnvironmentList.map(
            (environment) =>
              environment && (
                <EnvironmentCard
                  key={environment.name}
                  orgId={orgId ?? "default"}
                  projectId={projectId ?? "default"}
                  agentId={agentId ?? "default"}
                  environment={environment}
                />
              )
          )}
        </>
      )}
    </Box>
  );
};

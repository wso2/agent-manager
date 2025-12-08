import { Outlet, useMatch, useParams } from "react-router-dom";
// import { EnvBuildSelector } from "./EnvBuildSelector";
import {
  displayProvisionTypes,
  PageLayout,
} from "@agent-management-platform/views";
import { useGetAgent } from "@agent-management-platform/api-client";
import { Box, Chip, Skeleton } from "@wso2/oxygen-ui";
import { absoluteRouteMap } from "@agent-management-platform/types";

const AgentLayoutSkeleton = () => {
  return (
    <Box display="flex" flexDirection="column" gap={1} width="100%">
      <Box display="flex" flexDirection="row" gap={1} alignItems="center">
        <Skeleton variant="circular" width={50} height={50} />
        <Skeleton variant="rounded" width={400} height={50} />
      </Box>
      <Skeleton variant="rounded" width="100%" height="80vh" />
    </Box>
  );
};

export function AgentLayout() {
  return <Outlet />;
}

export interface AgentInfoPageLayoutProps {
  children: React.ReactNode;
}
export function AgentInfoPageLayout({ children }: AgentInfoPageLayoutProps) {
  const { orgId, agentId, projectId } = useParams();
  const { data: agent, isLoading: isAgentLoading } = useGetAgent({
    orgName: orgId ?? "default",
    projName: projectId ?? "default",
    agentName: agentId ?? "",
  });
  const isOverview = !!useMatch(
    absoluteRouteMap.children.org.children.projects.children.agents.path
  );

  if (isAgentLoading) {
    return <AgentLayoutSkeleton />;
  }
  return (
    <PageLayout
      title={agent?.displayName ?? "Agent"}
      description={
        agent?.description && isOverview
          ? agent.description
          : "No description provided."
      }
      titleTail={
        <Chip
          label={displayProvisionTypes(agent?.provisioning?.type)}
          color="default"
          size="small"
          variant="outlined"
        />
      }
    >
      {children}
    </PageLayout>
  );
}

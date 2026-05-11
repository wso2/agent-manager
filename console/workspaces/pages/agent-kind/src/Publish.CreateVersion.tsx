import React from "react";
import { generatePath, useParams } from "react-router-dom";
import { Box, Typography } from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const PublishCreateVersion: React.FC = () => {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.publish.path,
    { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" },
  );

  return (
    <PageLayout
      title="Create New Version"
      description="Publish a new version of this agent kind to the catalog."
      disableIcon
      backHref={backHref}
      backLabel="Back to Publish"
    >
      <Box
        display="flex"
        flexDirection="column"
        justifyContent="center"
        alignItems="center"
        minHeight="45vh"
      >
        <Typography variant="h5">Create New Version</Typography>
        <Typography variant="body2" color="text.secondary">
          Version creation form will be added here.
        </Typography>
      </Box>
    </PageLayout>
  );
};

export default PublishCreateVersion;

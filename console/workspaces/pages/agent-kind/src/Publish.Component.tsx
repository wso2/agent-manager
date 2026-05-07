import React from "react";
import { Box, Typography } from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";

export const PublishComponent: React.FC = () => {
  return (
    <PageLayout
      title="Publish"
      description="Manage and publish this agent kind to the catalog."
      disableIcon
    >
      <Box
        display="flex"
        flexDirection="column"
        justifyContent="center"
        alignItems="center"
        minHeight="45vh"
      >
        <Typography variant="h5">Publish Page</Typography>
        <Typography variant="body2" color="text.secondary">
          Agent Kind publishing controls will be added here.
        </Typography>
      </Box>
    </PageLayout>
  );
};

export default PublishComponent;

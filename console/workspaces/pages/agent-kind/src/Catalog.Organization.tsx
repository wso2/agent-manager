import React from "react";
import { Box, Typography } from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";

export const CatalogOrganization: React.FC = () => {
  return (
    <PageLayout
      title="Agent Catalog"
      description="Browse and manage cataloged agent kinds for this organization."
      disableIcon
    >
      <Box
        display="flex"
        flexDirection="column"
        justifyContent="center"
        alignItems="center"
        minHeight="45vh"
      >
        <Typography variant="h5">Catalog Page</Typography>
        <Typography variant="body2" color="text.secondary">
          Organization-level Agent Kind catalog features will be added here.
        </Typography>
      </Box>
    </PageLayout>
  );
};

export default CatalogOrganization;

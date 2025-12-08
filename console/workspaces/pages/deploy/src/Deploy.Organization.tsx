import React from 'react';
import { Box, Typography } from '@wso2/oxygen-ui';

export interface DeployOrganizationProps {
  title?: string;
  description?: string;
}

export const DeployOrganization: React.FC<DeployOrganizationProps> = ({
  title = 'Deploy - Organization Level',
  description = 'A page component for Deploy',
}) => {
  return (
    <Box sx={{ p: 3 }}>
      <Typography variant="h4" component="h1" gutterBottom>
        {title}
      </Typography>
      <Typography variant="body1" color="text.secondary">
        {description}
      </Typography>
      <Typography variant="caption" display="block" sx={{ mt: 2 }}>
        Organization Level View
      </Typography>
    </Box>
  );
};

export default DeployOrganization;

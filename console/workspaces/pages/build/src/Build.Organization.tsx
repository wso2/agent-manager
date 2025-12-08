import React from 'react';
import { Box, Typography } from '@wso2/oxygen-ui';

export interface BuildOrganizationProps {
  title?: string;
  description?: string;
}

export const BuildOrganization: React.FC<BuildOrganizationProps> = ({
  title = 'Build - Organization Level',
  description = 'A page component for Build',
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

export default BuildOrganization;

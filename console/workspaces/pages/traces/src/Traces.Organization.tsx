import React from 'react';
import { Box, Typography } from '@wso2/oxygen-ui';

export interface TracesOrganizationProps {
  title?: string;
  description?: string;
}

export const TracesOrganization: React.FC<TracesOrganizationProps> = ({
  title = 'Traces - Organization Level',
  description = 'A page component for Traces',
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

export default TracesOrganization;

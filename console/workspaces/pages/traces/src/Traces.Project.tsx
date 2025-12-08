import React from 'react';
import { Box, Typography } from '@wso2/oxygen-ui';

export interface TracesProjectProps {
  title?: string;
  description?: string;
}

export const TracesProject: React.FC<TracesProjectProps> = ({
  title = 'Traces - Project Level',
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
        Project Level View
      </Typography>
    </Box>
  );
};

export default TracesProject;

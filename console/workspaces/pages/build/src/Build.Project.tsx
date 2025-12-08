import React from 'react';
import { Box, Typography } from '@wso2/oxygen-ui';

export interface BuildProjectProps {
  title?: string;
  description?: string;
}

export const BuildProject: React.FC<BuildProjectProps> = ({
  title = 'Build - Project Level',
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
        Project Level View
      </Typography>
    </Box>
  );
};

export default BuildProject;

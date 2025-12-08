import React from 'react';
import { Box, Typography } from '@wso2/oxygen-ui';

export interface TestProjectProps {
  title?: string;
  description?: string;
}

export const TestProject: React.FC<TestProjectProps> = ({
  title = 'Test - Project Level',
  description = 'A page component for Test',
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

export default TestProject;

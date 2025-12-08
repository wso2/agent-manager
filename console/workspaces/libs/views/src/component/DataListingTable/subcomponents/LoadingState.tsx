import React from 'react';
import { Box, CircularProgress, Typography } from '@wso2/oxygen-ui';

export interface LoadingStateProps {
  message?: string;
  minHeight?: number;
}

export const LoadingState: React.FC<LoadingStateProps> = ({
  message = 'Loading...',
  minHeight = 200,
}) => {
  return (
    <Box 
      display="flex" 
      flexDirection="column"
      justifyContent="center" 
      alignItems="center" 
      minHeight={minHeight}
      gap={2}
      padding={4}
    >
      <CircularProgress size={40} />
      <Typography variant="body2">
        {message}
      </Typography>
    </Box>
  );
};

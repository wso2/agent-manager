import React from 'react';
import { Box } from '@wso2/oxygen-ui';
import { NoDataFound } from '../../NoDataFound/NoDataFound';

export const EmptyState: React.FC = () => {
  return (
    <Box 
      display="flex" 
      flexDirection="column"
      justifyContent="center" 
      alignItems="center" 
      minHeight={200}
      gap={2}
      padding={4}
    >
      <NoDataFound message="No data found" />
    </Box>
  );
};

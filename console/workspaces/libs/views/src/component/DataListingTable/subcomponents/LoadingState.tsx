/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from 'react';
import { Box, CircularProgress, Typography, useTheme } from '@mui/material';

export interface LoadingStateProps {
  message?: string;
  minHeight?: number;
}

export const LoadingState: React.FC<LoadingStateProps> = ({
  message = 'Loading...',
  minHeight = 200,
}) => {
  const theme = useTheme();

  return (
    <Box 
      display="flex" 
      flexDirection="column"
      justifyContent="center" 
      alignItems="center" 
      minHeight={minHeight}
      gap={theme.spacing(2)}
      padding={theme.spacing(4)}
    >
      <CircularProgress 
        size={40}
        sx={{
          color: theme.palette.primary.main,
        }}
      />
      <Typography 
        variant="body2" 
        sx={{
          color: theme.palette.text.secondary,
          fontSize: theme.typography.body2.fontSize,
        }}
      >
        {message}
      </Typography>
    </Box>
  );
};

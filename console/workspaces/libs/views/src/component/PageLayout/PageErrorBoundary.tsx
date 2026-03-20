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

import { Box, Button, Stack, Typography } from '@wso2/oxygen-ui';
import { RefreshCcw } from '@wso2/oxygen-ui-icons-react';
import { Component, ErrorInfo, ReactNode } from 'react';

export interface PageErrorBoundaryProps {
  children: ReactNode;
  title?: string;
  fullWidth: boolean;
}

interface PageErrorBoundaryState {
  hasError: boolean;
}

export class PageErrorBoundary extends Component<
  PageErrorBoundaryProps,
  PageErrorBoundaryState
> {
  state: PageErrorBoundaryState = {
    hasError: false,
  };

  static getDerivedStateFromError(): PageErrorBoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // Surface the error so observers can pick it up.
    console.error('PageLayout failed to render', error, errorInfo);
  }

  private handleRetry = () => {
    this.setState({ hasError: false });
  };


  render() {
    if (this.state.hasError) {
      return (
        <Box
          sx={{
            minHeight: '80vh',
            width: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: 4,
          }}
        >
          <Stack spacing={2} alignItems="center" sx={{ maxWidth: 480, textAlign: 'center' }}>
            <Typography variant="h1" color="text.secondary" sx={{ fontWeight: 700, fontSize: '6rem' }}>
              Oops!
            </Typography>
            <Typography variant="h5">
              Something went wrong.
            </Typography>
            <Typography variant="body1" color="text.secondary">
              An unexpected error has occurred. Please try again later.
            </Typography>
            <Button startIcon={<RefreshCcw size={16} />} variant="contained" color="primary" onClick={this.handleRetry}>
              Retry
            </Button>
          </Stack>
        </Box>
      );
    }

    return this.props.children;
  }
}

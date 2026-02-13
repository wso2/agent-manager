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

import { Box, Button, PageContent, Stack } from '@wso2/oxygen-ui';
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

  private handleReload = () => {
    if (typeof window !== 'undefined') {
      window.location.reload();
    }
  };

  render() {
    if (this.state.hasError) {
      return (
        <PageContent fullWidth={this.props.fullWidth}>
          <Box
            role="alert"
            sx={{
              borderRadius: 2,
              border: '1px solid',
              borderColor: 'divider',
              p: 4,
              display: 'flex',
              justifyContent: 'center',
              bgcolor: 'background.paper',
              minHeight: 240,
            }}
          >
            <Stack spacing={2} alignItems="center" textAlign="center">
              <Box component="h3" sx={{ m: 0, fontSize: 20 }}>
                {this.props.title
                  ? `${this.props.title} is unavailable`
                  : 'Something went wrong'}
              </Box>
              <Box component="p" sx={{ m: 0, color: 'text.secondary', maxWidth: 420 }}>
                We could not render this page. Try again or reload to continue.
              </Box>
              <Stack direction="row" spacing={1}>
                <Button variant="contained" onClick={this.handleRetry}>
                  Try again
                </Button>
                <Button variant="outlined" onClick={this.handleReload}>
                  Reload
                </Button>
              </Stack>
            </Stack>
          </Box>
        </PageContent>
      );
    }

    return this.props.children;
  }
}

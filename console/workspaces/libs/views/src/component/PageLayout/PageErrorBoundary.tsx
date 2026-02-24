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

import { Button, Card, CardContent, PageContent, Stack, Typography } from '@wso2/oxygen-ui';
import { Bug, RefreshCcw } from '@wso2/oxygen-ui-icons-react';
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
        <PageContent fullWidth={this.props.fullWidth}>
          <Card>
            <CardContent>
              <Stack spacing={3} alignItems="center" textAlign="center" py={4}>
                <Bug size={42} />
                <Stack spacing={0.5} alignItems="center">
                  <Typography variant="h6" fontWeight={600}>
                    Something went wrong
                  </Typography>
                  <Typography variant="body2" color="text.secondary" maxWidth={420}>
                    We could not render this page due to some unexpected reason.
                  </Typography>
                </Stack>
                <Stack direction="row" spacing={1}>
                  <Button startIcon={<RefreshCcw size={16} />} variant="contained" onClick={this.handleRetry}>
                    Retry
                  </Button>
                </Stack>
              </Stack>
            </CardContent>
          </Card>
        </PageContent>
      );
    }

    return this.props.children;
  }
}

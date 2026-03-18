/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { Component, type ErrorInfo, type ReactNode } from "react";
import { Alert, Button, Stack, Typography } from "@wso2/oxygen-ui";
import { RefreshCcw } from "@wso2/oxygen-ui-icons-react";

interface SectionErrorBoundaryProps {
  children: ReactNode;
  fallbackMessage?: string;
}

interface SectionErrorBoundaryState {
  hasError: boolean;
}

export class SectionErrorBoundary extends Component<
  SectionErrorBoundaryProps,
  SectionErrorBoundaryState
> {
  state: SectionErrorBoundaryState = { hasError: false };

  static getDerivedStateFromError(): SectionErrorBoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("Section render error", error, errorInfo);
  }

  private handleRetry = () => {
    this.setState({ hasError: false });
  };

  render() {
    if (this.state.hasError) {
      return (
        <Alert severity="error" sx={{ my: 1 }}>
          <Stack spacing={1} alignItems="flex-start">
            <Typography variant="body2">
              {this.props.fallbackMessage ??
                "This section failed to render."}
            </Typography>
            <Button
              size="small"
              variant="outlined"
              startIcon={<RefreshCcw size={14} />}
              onClick={this.handleRetry}
            >
              Retry
            </Button>
          </Stack>
        </Alert>
      );
    }

    return this.props.children;
  }
}

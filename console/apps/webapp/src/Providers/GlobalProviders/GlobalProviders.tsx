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

import { AuthProvider } from "@agent-management-platform/auth";
import { ClientProvider } from "@agent-management-platform/api-client";
import { ThemeProvider as MuiThemeProvider } from "@mui/material";
import { aiAgentTheme, aiAgentDarkTheme } from "@agent-management-platform/views";
import { ThemeProvider, useTheme } from "../../contexts/ThemeContext";

const MuiThemeWrapper = ({ children }: { children: React.ReactNode }) => {
  const { actualTheme } = useTheme();
  const theme = actualTheme === 'dark' ? aiAgentDarkTheme : aiAgentTheme;

  return <MuiThemeProvider theme={theme}>{children}</MuiThemeProvider>;
};

export const GlobalProviders = ({ children }: { children: React.ReactNode }) => {
  return (
    <ThemeProvider>
      <MuiThemeWrapper>
        <AuthProvider>
          <ClientProvider>
            {children}
          </ClientProvider>
        </AuthProvider>
      </MuiThemeWrapper>
    </ThemeProvider>
  );
};

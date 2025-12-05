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
import { Decorator } from '@storybook/react';
import { useDarkMode } from 'storybook-dark-mode';
import { BrowserRouter } from 'react-router-dom';
import { IntlProvider } from 'react-intl';
import { ThemeProvider } from '@mui/material/styles';
import { CssBaseline } from '@mui/material';
import relativeTime from 'dayjs/plugin/relativeTime';
import dayjs from 'dayjs';
import { aiAgentTheme, aiAgentDarkTheme } from '../src/theme';

dayjs.extend(relativeTime);

export const withTheme: Decorator = (Story) => {
  const isDark = useDarkMode();
  const theme = isDark ? aiAgentDarkTheme : aiAgentTheme;

  return (
    <BrowserRouter>
      <IntlProvider locale="en">
        <ThemeProvider theme={theme}>
          <CssBaseline />
          <Story />
        </ThemeProvider>
      </IntlProvider>
    </BrowserRouter>
  );
}; 

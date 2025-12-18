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

import { Box, Paper, Typography } from '@wso2/oxygen-ui';
import {
  LucideProps,
  SearchX as SearchOffOutlined,
} from '@wso2/oxygen-ui-icons-react';
import { FadeIn } from '../FadeIn/FadeIn';
import React, { createElement, ReactNode } from 'react';

interface NoDataFoundProps {
  message?: string;
  action?: ReactNode;
  icon?: ReactNode;
  iconElement?: React.ForwardRefExoticComponent<
    Omit<LucideProps, 'ref'> & React.RefAttributes<SVGSVGElement>
  >;
  subtitle?: string;
  disableBackground?: boolean;
}

export function NoDataFound({
  message = 'No data found',
  action,
  icon,
  iconElement,
  subtitle,
  disableBackground = false,
}: NoDataFoundProps) {
  const WrapperComponent = (props: { children: ReactNode }) =>
    disableBackground ? (
      <Box
        sx={{
          display: 'flex',
          height: '100%',
          width: '100%',
          justifyContent: 'center',
          alignItems: 'center',
          flexDirection: 'column',
          gap: 1,
          p: 4,
        }}
      >
        {props.children}
      </Box>
    ) : (
      <Paper
        variant="outlined"
        elevation={0}
        sx={{
          display: 'flex',
          height: '100%',
          width: '100%',
          justifyContent: 'center',
          alignItems: 'center',
          flexDirection: 'column',
          gap: 1,
          p: 4,
          '&.MuiPaper-root': {
            backgroundColor: 'background.default',
          },
        }}
      >
        {props.children}
      </Paper>
    );
  return (
    <FadeIn>
      <WrapperComponent>
        <Box color="secondary.dark">
          <Typography variant="body2" color="textSecondary">
            {iconElement
              ? createElement(iconElement, { size: 100 })
              : (icon ?? <SearchOffOutlined size={100} />)}
          </Typography>
        </Box>
        <Typography variant="h6">{message}</Typography>
        {subtitle && (
          <Typography variant="caption" color="textSecondary" align="center">
            {subtitle}
          </Typography>
        )}
        {action && <Box sx={{ mt: 2 }}>{action}</Box>}
      </WrapperComponent>
    </FadeIn>
  );
}

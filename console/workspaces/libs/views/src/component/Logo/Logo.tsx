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

import { Box, Typography, useColorScheme } from '@wso2/oxygen-ui';
import React from 'react';

export type LogoProps = React.SVGProps<SVGSVGElement>;

export function Logo(props: LogoProps) {
  const { colorScheme } = useColorScheme();
  const isDark = colorScheme === 'dark';
  const color = isDark ? '#ffffff' : '#000000';
  const markColor = isDark ? '#FFFFFF' : '#0B1633';
  return (
    <Box display="flex" justifyContent="center" alignItems="end">
      <svg width="28" height="28" viewBox="0 0 1334.78 1401.85" xmlns="http://www.w3.org/2000/svg" {...props}>
        {/* Choreo Labs mark: Midnight Ink in light mode, white in dark. */}
        <g fill={markColor}>
          <ellipse cx="1121.72" cy="446.86" rx="213.06" ry="96.05" />
          <ellipse cx="994.89" cy="214.11" rx="205.12" ry="81.09" />
          <ellipse cx="742.48" cy="38.71" rx="223.72" ry="38.71" />
          <ellipse cx="1121.72" cy="954.99" rx="213.06" ry="96.05" />
          <ellipse cx="994.89" cy="1187.74" rx="205.12" ry="81.09" />
          <ellipse cx="742.48" cy="1363.14" rx="223.72" ry="38.71" />
          <ellipse cx="213.06" cy="700.92" rx="213.06" ry="96.05" />
          <ellipse cx="287.14" cy="933.67" rx="173.98" ry="81.09" />
          <ellipse cx="287.14" cy="468.17" rx="173.98" ry="81.09" />
        </g>
      </svg>
      <Typography variant="h5" color={color} marginLeft={1} mb={0.2}>
        <Box component="span" fontWeight="bold">Choreo&nbsp;</Box>Agent Manager
      </Typography>
    </Box>
  );
}

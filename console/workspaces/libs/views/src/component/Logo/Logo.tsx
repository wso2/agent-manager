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

import { withBasePath } from '@agent-management-platform/types';
import { useColorScheme } from '@wso2/oxygen-ui';
import React from 'react';

export type LogoProps = Omit<React.ImgHTMLAttributes<HTMLImageElement>, 'src'>;

// Wordmark artwork lives in the web app's public/images. The "dark" file is
// the dark-ink variant for light backgrounds; "white" is for dark backgrounds.
const LOGO_FOR_LIGHT = '/images/choreo-ai-control-plane-dark.svg';
const LOGO_FOR_DARK = '/images/choreo-ai-control-plane-white.svg';

export function Logo({ alt = 'Choreo AI Control Plane', style, ...props }: LogoProps) {
  const { colorScheme } = useColorScheme();
  const src = withBasePath(colorScheme === 'dark' ? LOGO_FOR_DARK : LOGO_FOR_LIGHT);
  return <img {...props} src={src} alt={alt} style={{ height: 28, width: 'auto', ...style }} />;
}

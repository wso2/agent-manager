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

import { Box, PageTitle, PageContent } from '@wso2/oxygen-ui';
import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useDocumentTitle } from '../../hooks/useDocumentTitle';

export interface PageLayoutProps {
  children: ReactNode;
  backHref?: string;
  title?: string;
  backLabel?: string;
  description?: string;
  titleTail?: ReactNode;
  disableIcon?: boolean;
  actions?: ReactNode;
  disablePadding?: boolean;
}
export function PageLayout({
  children,
  title,
  backHref,
  backLabel,
  description,
  titleTail,
  actions,
  disablePadding = false,
  disableIcon = false,
}: PageLayoutProps) {
  useDocumentTitle(title);

  return (
        <PageContent sx={{flexGrow:1}} fullWidth= {!disablePadding}>
          <PageTitle>
            {backHref && (
              <PageTitle.BackButton component={<Link to={backHref} />}>
                {backLabel || 'Back'}
              </PageTitle.BackButton>
            )}

            {!disableIcon && (
              <PageTitle.Avatar>
                {title?.substring(0, 1).toUpperCase()}
              </PageTitle.Avatar>
            )}

            <PageTitle.Header>
              {title}
              {titleTail && (
                <Box
                  component="span"
                  sx={{ ml: 1, display: 'inline-flex', alignItems: 'center' }}
                >
                  {titleTail}
                </Box>
              )}
            </PageTitle.Header>

            {description && (
              <PageTitle.SubHeader>{description}</PageTitle.SubHeader>
            )}

            {actions && <PageTitle.Actions>{actions}</PageTitle.Actions>}
          </PageTitle>
            {children}
        </PageContent>
  
  );
}

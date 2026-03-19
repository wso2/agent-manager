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

import { Box, PageTitle, PageContent, Skeleton, Stack } from '@wso2/oxygen-ui';
import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useDocumentTitle } from '../../hooks/useDocumentTitle';
import { PageErrorBoundary } from './PageErrorBoundary';

export interface PageLayoutProps {
  children: ReactNode;
  backHref?: string;
  title?: string;
  backLabel?: string;
  description?: ReactNode;
  titleTail?: ReactNode;
  disableIcon?: boolean;
  actions?: ReactNode;
  disablePadding?: boolean;
  isLoading?: boolean;
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
  isLoading,
}: PageLayoutProps) {
  useDocumentTitle(title);


  if (isLoading) {
    return (
      <PageContent fullWidth={!disablePadding}>
        <PageTitle sx={{ mb: 4 }}>
          {backHref && (
            <PageTitle.BackButton component={<Link to={backHref} />}>
              {backLabel || 'Back'}
            </PageTitle.BackButton>
          )}
          {!disableIcon ? (
            <PageTitle.Avatar>
              <Skeleton variant="circular" width={80} height={80} />
            </PageTitle.Avatar>
          ) : null}
          <PageTitle.Header>
            <Skeleton variant="text" width={200} height={32} />
          </PageTitle.Header>
          {description && (
            <PageTitle.SubHeader>
              <Skeleton variant="text" width={300} height={20} />
            </PageTitle.SubHeader>
          )}
          {actions && (
            <PageTitle.Actions>
              <Stack direction="row" spacing={1}>
                <Skeleton variant="circular" width={32} height={32} />
                <Skeleton variant="circular" width={32} height={32} />
              </Stack>
            </PageTitle.Actions>
          )}
        </PageTitle>
        {children}
      </PageContent>
    );
  }

  return (
    <PageErrorBoundary title={title} fullWidth={!disablePadding}>
      <PageContent fullWidth={!disablePadding}>
        <PageTitle sx={{ mb: 4 }}>
          {backHref && (
            <PageTitle.BackButton component={<Link to={backHref} />}>
              {backLabel || 'Back'}
            </PageTitle.BackButton>
          )}
          {!disableIcon ? (
            <PageTitle.Avatar
              sx={{ bgcolor: 'primary.main', color: 'primary.contrastText' }}
              src={undefined}
            >
              {title?.substring(0, 1).toUpperCase()}
            </PageTitle.Avatar>
          ) : null}
          <PageTitle.Header>
            <Box
              component="span"
              sx={{
                display: 'inline-flex',
                alignItems: 'center',
                width: '100%',
              }}
            >
              {title}
              {titleTail && (
                <Box
                  component="span"
                  sx={{ display: 'inline-flex', alignItems: 'center', verticalAlign: 'middle', ml: 1 }}
                >
                  {titleTail}
                </Box>
              )}
            </Box>
          </PageTitle.Header>
          {description && (
            <PageTitle.SubHeader>{description}</PageTitle.SubHeader>
          )}
          {actions && <PageTitle.Actions>{actions}</PageTitle.Actions>}
        </PageTitle>
        {children}
      </PageContent>
    </PageErrorBoundary>
  );
}

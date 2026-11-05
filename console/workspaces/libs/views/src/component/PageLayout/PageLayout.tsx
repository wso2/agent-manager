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

import { PageContent } from '@wso2/oxygen-ui';
import type { ReactNode } from 'react';
import { useDocumentTitle } from '../../hooks/useDocumentTitle';
import { PageErrorBoundary } from './PageErrorBoundary';
import { PageHeader, type PageHeaderProps } from './PageHeader';

/** Page shell: a {@link PageHeader} over the page's content. */
export interface PageLayoutProps extends PageHeaderProps {
  children: ReactNode;
  /** @deprecated Pass `avatar={null}` instead. */
  disableIcon?: boolean;
  disablePadding?: boolean;
}

export function PageLayout({
  children,
  disablePadding = false,
  disableIcon = false,
  ...headerProps
}: PageLayoutProps) {
  const { title, avatar, isLoading } = headerProps;
  useDocumentTitle(title);

  const content = (
    <PageContent fullWidth noPadding={disablePadding}>
      <PageHeader {...headerProps} avatar={disableIcon ? null : avatar} />
      {children}
    </PageContent>
  );

  // While loading there's nothing for the boundary's retry to act on yet, so it
  // only wraps the settled page.
  if (isLoading) return content;

  return (
    <PageErrorBoundary title={title}>
      {content}
    </PageErrorBoundary>
  );
}

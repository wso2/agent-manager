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

import type { ReactElement, ReactNode } from 'react';
import { render, type RenderOptions } from '@testing-library/react';
import { OxygenUIThemeProvider } from '@wso2/oxygen-ui';
import { MemoryRouter, type MemoryRouterProps } from 'react-router-dom';

export interface ProviderOptions {
  /** Router history to start from; the tree always sits inside a MemoryRouter. */
  initialEntries?: MemoryRouterProps['initialEntries'];
}

export function Providers({
  children,
  initialEntries = ['/'],
}: ProviderOptions & { children: ReactNode }) {
  return (
    <OxygenUIThemeProvider>
      <MemoryRouter
        initialEntries={initialEntries}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >{children}</MemoryRouter>
    </OxygenUIThemeProvider>
  );
}

/** Renders `ui` inside the Oxygen theme and a MemoryRouter. */
export function renderWithProviders(
  ui: ReactElement,
  { initialEntries, ...options }: ProviderOptions & Omit<RenderOptions, 'wrapper'> = {},
) {
  return render(ui, {
    wrapper: ({ children }) => (
      <Providers initialEntries={initialEntries}>{children}</Providers>
    ),
    ...options,
  });
}

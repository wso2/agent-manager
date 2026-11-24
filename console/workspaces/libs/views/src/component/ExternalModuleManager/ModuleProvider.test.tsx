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

import type { ReactNode } from 'react';
import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import {
  ExternalModuleProvider,
  useAllModuleContext,
  useExternalComponentModules,
  useExternalComponentModulesByMountPoint,
  useExternalNavItems,
  useExternalPageModuleByMountPoint,
  useExternalPageModules,
  type ExternalModule,
} from './ModuleProvider';

const Noop = () => null;

const pageA: ExternalModule = {
  kind: 'page', mountPoint: 'agent.tab', moduleName: 'a', path: '/a', pageComponent: Noop,
};
const pageB: ExternalModule = {
  kind: 'page', mountPoint: 'org.page', moduleName: 'b', path: '/b', pageComponent: Noop,
};
const panelOverview: ExternalModule = {
  kind: 'component', mountPoint: 'overview', moduleName: 'c', component: Noop,
};
const panelSidebar: ExternalModule = {
  kind: 'component', mountPoint: 'sidebar', moduleName: 'd', component: Noop,
};
const nav: ExternalModule = {
  kind: 'nav-item', mountPoint: 'nav', moduleName: 'e', icon: null, title: 'E', route: '/e', level: 'org',
};
const all = [pageA, pageB, panelOverview, panelSidebar, nav];

const wrapper = ({ children }: { children: ReactNode }) => (
  <ExternalModuleProvider externalPageModules={all}>{children}</ExternalModuleProvider>
);

describe('ExternalModuleProvider hooks', () => {
  it('default to no modules outside a provider', () => {
    expect(renderHook(() => useAllModuleContext()).result.current.externalPageModules).toEqual([]);
    expect(renderHook(() => useExternalPageModules()).result.current).toEqual([]);
  });

  it('defaults to no modules when the provider is given none', () => {
    const { result } = renderHook(() => useExternalNavItems(), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <ExternalModuleProvider>{children}</ExternalModuleProvider>
      ),
    });
    expect(result.current).toEqual([]);
  });

  it('split modules by kind', () => {
    const { result } = renderHook(
      () => ({
        pages: useExternalPageModules(),
        components: useExternalComponentModules(),
        navItems: useExternalNavItems(),
      }),
      { wrapper },
    );
    expect(result.current.pages).toEqual([pageA, pageB]);
    expect(result.current.components).toEqual([panelOverview, panelSidebar]);
    expect(result.current.navItems).toEqual([nav]);
  });

  it('filter component modules by mount point', () => {
    const { result } = renderHook(
      () => ({
        byId: useExternalComponentModules('sidebar'),
        byMount: useExternalComponentModulesByMountPoint('overview'),
        allByEmptyMount: useExternalComponentModulesByMountPoint(''),
        none: useExternalComponentModulesByMountPoint('nowhere'),
      }),
      { wrapper },
    );
    expect(result.current.byId).toEqual([panelSidebar]);
    expect(result.current.byMount).toEqual([panelOverview]);
    expect(result.current.allByEmptyMount).toEqual([panelOverview, panelSidebar]);
    expect(result.current.none).toEqual([]);
  });

  it('find the page module at a mount point, ignoring other kinds', () => {
    const { result } = renderHook(
      () => ({
        page: useExternalPageModuleByMountPoint('org.page'),
        panelMount: useExternalPageModuleByMountPoint('overview'),
      }),
      { wrapper },
    );
    expect(result.current.page).toBe(pageB);
    expect(result.current.panelMount).toBeUndefined();
  });
});

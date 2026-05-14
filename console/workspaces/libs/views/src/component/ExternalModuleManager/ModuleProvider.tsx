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

import { createContext, useContext, useMemo } from 'react';

export interface ExternalModuleCore {
  mountPoint: string;
  moduleName: string;
}

export interface ExternalPageModule extends ExternalModuleCore {
  pageComponent: React.ComponentType<Record<string, unknown>>;
  kind: 'page';
  path: string;
  icon?: React.ReactNode;
}

export interface ExternalComponentModule extends ExternalModuleCore {
  component: React.ComponentType<Record<string, unknown>>;
  kind: 'component';
}

export interface ExternalConfigModule extends ExternalModuleCore {
  value: object;
  kind: 'config';
}

export interface ExternalNavItem extends ExternalModuleCore {
  icon: React.ReactNode;
  title: string;
  route: string;
  moduleName: string;
  level: 'project' | 'org' | 'component';
  kind: 'nav-item';
}

export type ExternalModule =
  | ExternalPageModule
  | ExternalComponentModule
  | ExternalNavItem
  | ExternalConfigModule;

export interface ModuleContextValue {
  externalPageModules?: ExternalModule[];
}
const ModuleContext = createContext({
  externalPageModules: [],
} as ModuleContextValue);

export interface ModuleProviderProps {
  children: React.ReactNode;
  externalPageModules?: ExternalModule[];
}

export function ExternalModuleProvider({
  children,
  externalPageModules,
}: ModuleProviderProps) {
  return (
    <ModuleContext.Provider value={{ externalPageModules }}>
      {children}
    </ModuleContext.Provider>
  );
}

export function useAllModuleContext() {
  const context = useContext(ModuleContext);
  if (!context) {
    throw new Error('useAllModuleContext must be used within a ModuleProvider');
  }
  return context;
}

export function useExternalPageModules() {
  const { externalPageModules } = useAllModuleContext();
  const modules = useMemo(
    () => externalPageModules?.filter((module) => module.kind === 'page') || [],
    [externalPageModules]
  );
  return modules as ExternalPageModule[];
}

export function useExternalConfigModules(mountPoint?: string) {
  const { externalPageModules } = useAllModuleContext();
  const modules = useMemo(() => {
    if (mountPoint) {
      return (
        externalPageModules?.filter(
          (module) =>
            module.kind === 'config' && module.mountPoint === mountPoint
        ) || []
      );
    }
    return (
      externalPageModules?.filter((module) => module.kind === 'config') || []
    );
  }, [externalPageModules, mountPoint]);
  return modules as ExternalConfigModule[];
}

export function useExternalComponentModules(id?: string) {
  const { externalPageModules } = useAllModuleContext();
  const modules = useMemo(() => {
    if (id) {
      return externalPageModules?.filter(
        (module) => module.kind === 'component' && module.mountPoint === id
      );
    }
    return (
      externalPageModules?.filter((module) => module.kind === 'component') || []
    );
  }, [externalPageModules, id]);
  return modules as ExternalComponentModule[];
}

export function useExternalNavItems() {
  const { externalPageModules } = useAllModuleContext();
  const modules = useMemo(
    () =>
      externalPageModules?.filter((module) => module.kind === 'nav-item') || [],
    [externalPageModules]
  );
  return modules as ExternalNavItem[];
}

export function useExternalPageModuleByMountPoint(mountPoint: string) {
  const { externalPageModules } = useAllModuleContext();
  const module = useMemo(
    () =>
      externalPageModules?.find(
        (m) => m.kind === 'page' && m.mountPoint === mountPoint
      ),
    [externalPageModules, mountPoint]
  );
  return module;
}

export function useExternalComponentModulesByMountPoint(mountPoint: string) {
  const { externalPageModules } = useAllModuleContext();
  const modules = useMemo(() => {
    if (mountPoint) {
      return externalPageModules?.filter(
        (module) =>
          module.kind === 'component' && module.mountPoint === mountPoint
      );
    }
    return (
      externalPageModules?.filter((module) => module.kind === 'component') || []
    );
  }, [externalPageModules, mountPoint]);
  return modules;
}

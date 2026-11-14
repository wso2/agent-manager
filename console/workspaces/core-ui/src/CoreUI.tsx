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

import { useMemo } from 'react';
import type { ExternalModule } from '@agent-management-platform/views';
import type { FeatureFlags } from '@agent-management-platform/feature-management';
import type { UIModuleManifest } from '@agent-management-platform/types';
import type { UseFeatureAccess } from './types/featureAccess';
import { GlobalProviders } from './Providers';
import { RootRouter } from './Route';
import { builtinModules } from './plugins';

export interface CoreUIProps {
  externalPageModules?: ExternalModule[];
  /**
   * Extra UI modules to register next to the built-in ones. Pass a stable array
   * (a module constant), not a new one on every render.
   */
  modules?: UIModuleManifest[];
  /** Feature flags; override the runtime config's `featureFlags`. */
  featureFlags?: Partial<FeatureFlags>;
  /**
   * Hook returning the user's access. It runs inside the auth providers, so it can
   * read the token. Pass the same function on every render. Omit to allow everything.
   */
  useFeatureAccess?: UseFeatureAccess;
}

function CoreUI({ externalPageModules, modules, featureFlags, useFeatureAccess }: CoreUIProps) {
  const allModules = useMemo(
    () => (modules?.length ? [...builtinModules, ...modules] : builtinModules),
    [modules],
  );
  return (
    <GlobalProviders
      externalPageModules={externalPageModules}
      modules={allModules}
      featureFlags={featureFlags}
      useFeatureAccess={useFeatureAccess}
    >
      <RootRouter />
    </GlobalProviders>
  )
}

export default CoreUI

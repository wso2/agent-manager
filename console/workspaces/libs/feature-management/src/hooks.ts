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

import { useFeatureManagement } from "./FeatureManagementProvider";
import type { AccessResource, FeatureAccessKey, FeatureFlagKey } from "./types";

export function useFeatureFlag(key: FeatureFlagKey): boolean {
  return useFeatureManagement().isFlagOn(key);
}

export function useFeatureAccess(key: FeatureAccessKey): boolean {
  return useFeatureManagement().hasAccess(key);
}

export interface ResourceAccess {
  canRead: boolean;
  canWrite: boolean;
}

/** Read and write access to one resource, e.g. `useResourceAccess("agent")`. */
export function useResourceAccess(resource: AccessResource): ResourceAccess {
  const { hasAccess } = useFeatureManagement();
  return {
    canRead: hasAccess(`${resource}_read` as FeatureAccessKey),
    canWrite: hasAccess(`${resource}_write` as FeatureAccessKey),
  };
}

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

import { useFeatureManagement } from "../FeatureManagementProvider";
import type { AccessResource, FeatureAccessKey, FeatureFlagKey } from "../types";

export interface ManagedResourceFeatures {
  /** Show the nav item and pages. */
  canView: boolean;
  /** Show create, edit and delete actions. */
  canWrite: boolean;
}

/** Pages show unless managed outside the console, and need read; actions also need write. */
function useManagedResourceFeatures(
  managedOutsideFlag: FeatureFlagKey,
  resource: AccessResource,
): ManagedResourceFeatures {
  const { isFlagOn, hasAccess } = useFeatureManagement();
  const canView =
    !isFlagOn(managedOutsideFlag) && hasAccess(`${resource}_read` as FeatureAccessKey);
  return {
    canView,
    canWrite: canView && hasAccess(`${resource}_write` as FeatureAccessKey),
  };
}

export const useEnvironmentFeatures = () =>
  useManagedResourceFeatures("externallyManagedEnv", "env");

export const useGatewayFeatures = () =>
  useManagedResourceFeatures("externallyManagedGateway", "gateway");

export const usePipelineFeatures = () =>
  useManagedResourceFeatures("externallyManagedPipeline", "pipeline");

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
import { useResourceAccess } from "../hooks";
import type { AccessResource, FeatureFlagKey } from "../types";

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
  const { isFlagOn } = useFeatureManagement();
  const { canRead, canWrite } = useResourceAccess(resource);
  const canView = !isFlagOn(managedOutsideFlag) && canRead;
  return { canView, canWrite: canView && canWrite };
}

export function useGatewayFeatures(): ManagedResourceFeatures & { canManageTokens: boolean } {
  const features = useManagedResourceFeatures("externallyManagedGateway", "gateway");
  const { hasAccess } = useFeatureManagement();
  // Registration tokens have their own permission, separate from gateway write.
  return { ...features, canManageTokens: features.canView && hasAccess("gateway_token_write") };
}

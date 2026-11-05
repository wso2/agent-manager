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

import type { FeatureAccessConfig } from "@agent-management-platform/feature-management";

export interface FeatureAccessResult {
  /** The user's access; unset keys are allowed. */
  featureAccess?: Partial<FeatureAccessConfig>;
  /** True while access is still being worked out, e.g. the token is decoding. */
  isLoading: boolean;
}

/** A hook the embedding app passes to CoreUI; it runs inside the auth providers. */
export type UseFeatureAccess = () => FeatureAccessResult;

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

import { createContext, useContext, useMemo, type ReactNode } from "react";
import type {
  FeatureAccessConfig,
  FeatureEnableConfig,
  FeatureManagement,
} from "./types";

function createFeatureManagement(
  featureEnable: Partial<FeatureEnableConfig>,
  featureAccess: Partial<FeatureAccessConfig>,
  isAccessLoading: boolean,
): FeatureManagement {
  return {
    featureEnable,
    featureAccess,
    isAccessLoading,
    isDisabled: (key) => featureEnable[key] ?? false,
    hasAccess: (key) => featureAccess[key] ?? true,
  };
}

// Without a provider everything is enabled and accessible.
const FeatureManagementContext = createContext<FeatureManagement>(
  createFeatureManagement({}, {}, false),
);

export interface FeatureManagementProviderProps {
  /** Feature switches; unset keys are `false` (feature on). */
  featureEnable?: Partial<FeatureEnableConfig>;
  /** Access rights; unset keys are `true` (allowed). */
  featureAccess?: Partial<FeatureAccessConfig>;
  /** True while `featureAccess` is still being worked out, e.g. the token is decoding. */
  isAccessLoading?: boolean;
  children: ReactNode;
}

/** Nested providers inherit from the outer one and override only the keys they set. */
export function FeatureManagementProvider({
  featureEnable,
  featureAccess,
  isAccessLoading,
  children,
}: FeatureManagementProviderProps) {
  const parent = useContext(FeatureManagementContext);
  const value = useMemo(
    () =>
      createFeatureManagement(
        { ...parent.featureEnable, ...featureEnable },
        { ...parent.featureAccess, ...featureAccess },
        isAccessLoading ?? parent.isAccessLoading,
      ),
    [parent, featureEnable, featureAccess, isAccessLoading],
  );
  return (
    <FeatureManagementContext.Provider value={value}>
      {children}
    </FeatureManagementContext.Provider>
  );
}

export function useFeatureManagement(): FeatureManagement {
  return useContext(FeatureManagementContext);
}

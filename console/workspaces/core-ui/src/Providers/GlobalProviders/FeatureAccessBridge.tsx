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

import { useMemo, type ReactNode } from "react";
import { globalConfig } from "@agent-management-platform/types";
import {
  FeatureManagementProvider,
  type FeatureFlags,
} from "@agent-management-platform/feature-management";
import { FullPageLoader } from "@agent-management-platform/views";
import type { UseFeatureAccess } from "../../types/featureAccess";

const allowAll: UseFeatureAccess = () => ({ isLoading: false });

interface FeatureAccessBridgeProps {
  featureFlags?: Partial<FeatureFlags>;
  /** Must be the same function on every render, like any hook. */
  useAccess?: UseFeatureAccess;
  children: ReactNode;
}

/** Runs the app's access hook inside the auth providers and feeds feature management. */
export function FeatureAccessBridge({
  featureFlags,
  useAccess = allowAll,
  children,
}: FeatureAccessBridgeProps) {
  const { featureAccess, isLoading } = useAccess();
  // Props from the embedding app win over the runtime config.
  const resolvedFeatureFlags = useMemo(
    () => ({ ...globalConfig.featureFlags, ...featureFlags }),
    [featureFlags],
  );
  // Hold the app until access is known: hasAccess allows unset keys, so rendering
  // now would flash (or auto-run) actions the user can't perform.
  if (isLoading) return <FullPageLoader />;
  return (
    <FeatureManagementProvider
      featureFlags={resolvedFeatureFlags}
      featureAccess={featureAccess}
      isAccessLoading={isLoading}
    >
      {children}
    </FeatureManagementProvider>
  );
}

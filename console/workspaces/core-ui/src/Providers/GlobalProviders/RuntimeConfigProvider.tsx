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

import { useEffect, useState } from "react";
import {
  useRuntimeConfig,
  setObserverBaseUrl,
  setConsoleAnalyticsEnabled,
} from "@agent-management-platform/api-client";
import { FullPageLoader } from "@agent-management-platform/views";

export function RuntimeConfigProvider({ children }: { children: React.ReactNode }) {
  const { data, isLoading, isError } = useRuntimeConfig();
  // Children may read isObserverConfigured() synchronously in their render
  // body, so the store must be populated before they ever mount. Gating on
  // this flag (set in the same effect that writes the store) guarantees that;
  // gating on isLoading alone would let children's first render race ahead of
  // the effect and see the unset store.
  const [synced, setSynced] = useState(false);

  useEffect(() => {
    // Settle once discovery resolves OR errors. A hung config endpoint is
    // turned into an error by the fetch timeout (see getRuntimeConfig), so
    // isLoading always flips eventually — the app never traps on the loader.
    // On error data is undefined, so the store stays unset and pages render
    // the "observer not configured" state.
    if (!isLoading) {
      setObserverBaseUrl(data?.observerBaseUrl);
      // The service's CONSOLE_ANALYTICS_ENABLED decides this; the console has
      // no flag of its own. On error data is undefined, which leaves analytics
      // off — the safe direction for telemetry.
      setConsoleAnalyticsEnabled(data?.consoleAnalyticsEnabled);
      setSynced(true);
    }
  }, [isLoading, isError, data?.observerBaseUrl, data?.consoleAnalyticsEnabled]);

  // Gate rendering until discovery settles so observability pages never
  // render against a transiently-empty observer URL. On error/timeout we
  // proceed unconfigured — pages show the "observer not configured" state.
  if (!synced) return <FullPageLoader />;
  return <>{children}</>;
}

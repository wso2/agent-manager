/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { useEffect } from "react";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";
import type { BuildDetailsResponse } from "@agent-management-platform/types";

// Only the newest builds can still be in flight, so a small page is enough.
const RECENT_BUILDS_LIMIT = 20;

export const BUILD_IN_PROGRESS_REASON =
  "A build is in progress for this agent. Configuration changes are available once it finishes.";

const isBuildInFlight = (build: BuildDetailsResponse) =>
  build.status === "Pending" || build.status === "Running";

// useGetAgentBuilds only polls while a build is in flight. While idle, check at
// this slower rate so a build started from another tab or session is noticed.
const IDLE_DISCOVERY_INTERVAL_MS = 10_000;

// The service rejects creating or updating an agent configuration while a build
// is running (409). This mirrors that check so the console can block the action
// up front. useGetAgentBuilds keeps polling while a build is in flight, so the
// flag clears on its own once the build reaches a terminal state.
export function useHasBuildInProgress(
  params: { orgName?: string; projName?: string; agentName?: string },
  options?: { enabled?: boolean },
): boolean {
  const { data, isLoading, refetch } = useGetAgentBuilds(
    {
      orgName: params.orgName ?? "",
      projName: params.projName ?? "",
      agentName: params.agentName ?? "",
    },
    { limit: RECENT_BUILDS_LIMIT, offset: 0 },
    options,
  );
  const hasBuildInProgress = data?.builds?.some(isBuildInFlight) ?? false;

  // refetch() ignores the query's `enabled` flag, so mirror it here.
  const canQuery =
    (options?.enabled ?? true) &&
    !!params.orgName &&
    !!params.projName &&
    !!params.agentName;

  useEffect(() => {
    if (!canQuery || hasBuildInProgress) return undefined;
    const id = window.setInterval(() => {
      if (document.visibilityState === "visible") void refetch();
    }, IDLE_DISCOVERY_INTERVAL_MS);
    return () => window.clearInterval(id);
  }, [canQuery, hasBuildInProgress, refetch]);

  // Treat the first load as blocking so a save cannot slip through to the
  // service's 409 before the build list arrives. Gating on canQuery keeps cached
  // data from blocking agents this check does not apply to.
  return canQuery && (isLoading || hasBuildInProgress);
}

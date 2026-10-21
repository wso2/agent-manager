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

import { useCallback } from "react";
import {
  useListEnvironments,
  useTokenScopes,
} from "@agent-management-platform/api-client";

import {
  evaluateAgentEnvironmentAccess,
  type EnvironmentTier,
} from "./environmentTier";
import type { AccessDecision } from "./types";

/**
 * Hook form of {@link evaluateAgentEnvironmentAccess}, bound to the current
 * token and to `orgName`'s environments.
 *
 * The target may be named instead of passed as an object. Turning a name into a
 * tier was the same useListEnvironments lookup at every call site, so it lives
 * here: a caller that already holds the environment passes it, and one that
 * holds only the name — a promotion target, a ?deployPanel link's environment —
 * passes that. The list is already in the query cache, since the Deploy page
 * fetches it to lay its cards out, so naming the environment costs no request.
 *
 * Returns a stable callback so a caller can evaluate several environments — the
 * promotion targets of one card, say — in a single render.
 */
export function useAgentEnvironmentAccess(
  orgName: string | undefined,
): (
  environment: EnvironmentTier | string | undefined,
  capability?: string,
) => AccessDecision {
  const state = useTokenScopes();
  const { data: environments } = useListEnvironments({ orgName });
  return useCallback(
    (environment: EnvironmentTier | string | undefined, capability?: string) =>
      evaluateAgentEnvironmentAccess(
        state,
        typeof environment === "string"
          ? environments?.find((e) => e.name === environment)
          : environment,
        capability,
      ),
    [state, environments],
  );
}

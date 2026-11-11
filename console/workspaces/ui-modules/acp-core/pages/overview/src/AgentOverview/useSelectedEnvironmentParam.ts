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

import { useSearchParams } from "react-router-dom";
import type { Environment } from "@agent-management-platform/types";

export const ENV_SEARCH_PARAM = "env";

/**
 * Tracks the selected environment (by name) in the `env` search param instead
 * of local state, so the tab a user is on survives reloads/back-navigation.
 * Falls back to a default environment when the param is missing or points at
 * an environment outside the current list — the rightmost (most-promoted)
 * environment that `isDeployed` reports as deployed, so users land on
 * whatever's furthest along the pipeline instead of always Dev. Falls back
 * further to the first environment when none are deployed yet. When no
 * `isDeployed` predicate is given (e.g. external agents, which have no
 * platform-managed deployment concept to check), there's nothing to filter
 * on, so it defaults straight to the rightmost (most-promoted) environment.
 */
export function useSelectedEnvironmentParam(
  environments: Environment[],
  isDeployed?: (env: Environment) => boolean,
) {
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedName = searchParams.get(ENV_SEARCH_PARAM);
  // `??` short-circuits, so the reverse-scan for a default only actually runs
  // when the requested (or no) param fails to resolve directly — the common
  // case, once a selection is in the URL, skips it entirely.
  const selectedEnvironment =
    environments.find((env) => env.name === requestedName) ??
    (isDeployed
      ? [...environments].reverse().find(isDeployed) ?? environments[0]
      : environments[environments.length - 1]);

  const selectEnvironment = (name: string) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set(ENV_SEARCH_PARAM, name);
        return next;
      },
      { replace: true },
    );
  };

  return { selectedEnvironment, selectEnvironment };
}

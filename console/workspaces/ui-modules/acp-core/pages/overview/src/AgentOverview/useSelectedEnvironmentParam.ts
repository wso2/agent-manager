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
 * Falls back to the last environment in the list when the param is missing or
 * points at an environment outside the current list.
 */
export function useSelectedEnvironmentParam(environments: Environment[]) {
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedName = searchParams.get(ENV_SEARCH_PARAM);
  const selectedEnvironment =
    environments.find((env) => env.name === requestedName) ??
    environments[environments.length - 1];

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

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useMemo } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { useListEnvironments } from "@agent-management-platform/api-client";
import { useAgentIdentityEnvironment } from "../../context/AgentIdentityEnvironmentContext";

/**
 * Shared data/state for the org-level Agents/Roles/Groups environment
 * pickers — every environment in the org, the current `envName` search
 * param, and a change handler that updates it (and remembers it for the
 * session). Presentational components (dropdown, tabs, ...) render on top
 * of this without each re-implementing the fetch/param plumbing.
 */
export function useAgentIdentityEnvironmentOptions() {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const { setLastEnvName } = useAgentIdentityEnvironment();

  const envName = searchParams.get("envName") ?? "";
  const { data: environments } = useListEnvironments({ orgName: orgId });
  const options = useMemo(() => environments ?? [], [environments]);

  const handleChange = (newEnvName: string) => {
    setLastEnvName(newEnvName);
    const next = new URLSearchParams(searchParams);
    next.set("envName", newEnvName);
    setSearchParams(next);
  };

  return { envName, options, handleChange };
}

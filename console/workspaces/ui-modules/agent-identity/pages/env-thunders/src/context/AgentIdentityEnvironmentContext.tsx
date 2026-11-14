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

import React, { createContext, useContext, useEffect, useMemo, useState } from "react";
import { useParams, useSearchParams } from "react-router-dom";
import { useListEnvironments } from "@agent-management-platform/api-client";

interface AgentIdentityEnvironmentContextValue {
  lastEnvName: string | undefined;
  setLastEnvName: (envName: string) => void;
}

const AgentIdentityEnvironmentContext =
  createContext<AgentIdentityEnvironmentContextValue | undefined>(undefined);

export const AgentIdentityEnvironmentProvider: React.FC<{ children: React.ReactNode }> = ({
  children,
}) => {
  const [lastEnvName, setLastEnvName] = useState<string | undefined>(undefined);

  const value = useMemo(() => ({ lastEnvName, setLastEnvName }), [lastEnvName]);

  return (
    <AgentIdentityEnvironmentContext.Provider value={value}>
      {children}
    </AgentIdentityEnvironmentContext.Provider>
  );
};

export function useAgentIdentityEnvironment(): AgentIdentityEnvironmentContextValue {
  const context = useContext(AgentIdentityEnvironmentContext);
  if (!context) {
    throw new Error(
      "useAgentIdentityEnvironment must be used within an AgentIdentityEnvironmentProvider",
    );
  }
  return context;
}

interface ResolvedAgentIdentityEnvName {
  /** The current `envName` search param, if present and valid for this org. */
  envName: string | undefined;
  /** The environment to redirect to when `envName` is missing from the URL. */
  defaultEnvName: string | undefined;
  /** True once environments have loaded successfully and there isn't one to default to. */
  hasNoEnvironments: boolean;
  /** True when the environments list failed to load. */
  hasEnvironmentsError: boolean;
  /** Re-runs the environments query; exposed for an error/retry UI. */
  refetchEnvironments: () => void;
}

/**
 * Reads the `envName` search param for the org-level Agents/Roles/Groups
 * pages, and resolves a fallback (the last environment picked elsewhere in
 * Agent Identity this session, or the org's first environment) for callers
 * to redirect to when it's missing.
 */
export function useAgentIdentityEnvName(): ResolvedAgentIdentityEnvName {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const { lastEnvName, setLastEnvName } = useAgentIdentityEnvironment();
  const { data: environments, isLoading, isError, refetch } = useListEnvironments({
    orgName: orgId,
  });

  // `lastEnvName` lives in a provider that isn't remounted when the org
  // changes, and the `envName` search param can be stale or edited by hand —
  // only trust either once it's confirmed to belong to this org's
  // currently-loaded environments.
  const isKnownEnvName = (name: string | undefined): name is string =>
    !!name && (environments ?? []).some((environment) => environment.name === name);

  const rawEnvName = searchParams.get("envName") ?? undefined;
  const envName = isKnownEnvName(rawEnvName) ? rawEnvName : undefined;
  const defaultEnvName = isKnownEnvName(lastEnvName) ? lastEnvName : environments?.[0]?.name;

  useEffect(() => {
    if (envName) {
      setLastEnvName(envName);
    }
  }, [envName, setLastEnvName]);

  return {
    envName,
    defaultEnvName,
    hasNoEnvironments: !isLoading && !isError && !defaultEnvName,
    hasEnvironmentsError: isError,
    refetchEnvironments: refetch,
  };
}

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

import type { ReactNode } from "react";
import { Button, ListingTable } from "@wso2/oxygen-ui";
import { AlertTriangle, KeyRound } from "@wso2/oxygen-ui-icons-react";
import { Navigate, useLocation, useSearchParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { useAgentIdentityEnvName } from "../../context/AgentIdentityEnvironmentContext";

interface AgentIdentityEnvironmentGateProps {
  title: string;
  children: (envName: string) => ReactNode;
}

/**
 * Resolves the `envName` search param for the org-level Agents/Roles/Groups
 * pages before rendering `children` — redirects to a default environment
 * when it's missing, or shows an empty state if the org has none yet.
 */
export function AgentIdentityEnvironmentGate({
  title,
  children,
}: AgentIdentityEnvironmentGateProps) {
  const { pathname } = useLocation();
  const [searchParams] = useSearchParams();
  const { envName, defaultEnvName, hasNoEnvironments, hasEnvironmentsError, refetchEnvironments } =
    useAgentIdentityEnvName();

  if (hasEnvironmentsError) {
    return (
      <PageLayout title={title} disableIcon>
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<AlertTriangle size={64} />}
            title="Failed to load environments"
            description="Something went wrong while loading environments for this organization."
            action={
              <Button variant="contained" color="primary" onClick={() => refetchEnvironments()}>
                Retry
              </Button>
            }
          />
        </ListingTable.Container>
      </PageLayout>
    );
  }

  if (hasNoEnvironments) {
    return (
      <PageLayout title={title} disableIcon>
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<KeyRound size={64} />}
            title="No environments yet"
            description="Add an environment first to manage agent identity."
          />
        </ListingTable.Container>
      </PageLayout>
    );
  }

  if (!envName) {
    if (!defaultEnvName) {
      return (
        <PageLayout title={title} disableIcon isLoading>
          {null}
        </PageLayout>
      );
    }
    const next = new URLSearchParams(searchParams);
    next.set("envName", defaultEnvName);
    return <Navigate to={`${pathname}?${next.toString()}`} replace />;
  }

  return children(envName);
}

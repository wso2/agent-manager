/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React from "react";
import { Link, generatePath, useParams } from "react-router-dom";
import { Alert, Button, ListingTable, Skeleton } from "@wso2/oxygen-ui";
import { KeyRound, Rocket, ShieldOff } from "@wso2/oxygen-ui-icons-react";
import {
  useCreateAgentAPIKey,
  useGetAgentConfigurations,
  useListAgentDeployments,
  useListAgentAPIKeys,
  useRevokeAgentAPIKey,
} from "@agent-management-platform/api-client";
import {
  APIKeysManager,
  EnvironmentSelector,
  type CreateAPIKeyInput,
} from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";

export const SecurityComponent: React.FC = () => {
  const { orgId, projectId, agentId, envId } = useParams();

  // GetAgent returns only the lowest environment's config, so read per-env here.
  const { data: envConfig, isLoading: isLoadingConfig } =
    useGetAgentConfigurations(
      {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      },
      {
        environment: envId ?? "",
      },
    );
  const { data: deployments, isLoading: isLoadingDeployments } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });

  const securityEnabled = envConfig?.enableApiKeySecurity ?? true;
  const oauthEnabled = envConfig?.enableOAuthSecurity ?? false;
  const currentDeployment = envId ? deployments?.[envId] : undefined;
  const hasActiveDeployment = currentDeployment?.status === "active";
  const shouldLoadKeys =
    !isLoadingConfig &&
    !isLoadingDeployments &&
    hasActiveDeployment &&
    securityEnabled &&
    !!envId;
  const {
    data: keys,
    isLoading: isLoadingKeys,
    isError,
  } = useListAgentAPIKeys({
    orgName: shouldLoadKeys ? orgId : undefined,
    projName: shouldLoadKeys ? projectId : undefined,
    agentName: shouldLoadKeys ? agentId : undefined,
    envId: shouldLoadKeys ? envId : undefined,
  });
  const isLoading =
    isLoadingConfig || isLoadingDeployments || (shouldLoadKeys && isLoadingKeys);

  const { mutateAsync: createKey, isPending: isCreating } =
    useCreateAgentAPIKey();
  const { mutate: revokeKey, isPending: isRevoking } = useRevokeAgentAPIKey();
  const [gatewayOffline, setGatewayOffline] = React.useState(false);

  const handleCreate = async ({ displayName, expiresAt }: CreateAPIKeyInput) => {
    const data = await createKey({
      params: {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
        envId,
      },
      body: { displayName, expiresAt },
    });
    setGatewayOffline(data.gatewayConnected === false);
    return data.apiKey;
  };

  const handleRevoke = (keyName: string) => {
    revokeKey({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
      envId,
      keyName,
    });
  };

  const emptyState = !hasActiveDeployment
    ? {
        illustration: <Rocket size={48} />,
        title: "Agent is not deployed",
        description: "Deploy this agent to manage its API keys. Use the Deploy tab to get started.",
      }
    : !securityEnabled && oauthEnabled
    ? {
        illustration: <KeyRound size={48} />,
        title: "This agent uses OAuth",
        description: "Manage OAuth authentication from the configured identity provider.",
        action: orgId && envId ? (
          <Button
            variant="outlined"
            component={Link}
            to={generatePath(
              absoluteRouteMap.children.org.children.environments.children.view.children
                .identityProvider.path,
              { orgId, envName: envId },
            )}
          >
            View Identity Provider
          </Button>
        ) : undefined,
      }
    : !securityEnabled
    ? {
        illustration: <ShieldOff size={48} />,
        title: "Agent security is disabled",
        description: "Enable API Key Security from the deployment settings and redeploy to manage API keys.",
        action: orgId && projectId && agentId && envId ? (
          <Button
            variant="contained"
            component={Link}
            to={generatePath(
              absoluteRouteMap.children.org.children.projects.children.agents.children.environment
                .children.deploy.path,
              { orgId, projectId, agentId, envId },
            )}
          >
            Go to Deployment Settings
          </Button>
        ) : undefined,
      }
    : null;

  return (
    <PageLayout docs={["secureAgentsWithApiKeys", "secureAgentsWithJwt", "configureCors"]} title="API Keys" disableIcon actions={<EnvironmentSelector />}>
      {isLoading ? (
        <Skeleton variant="rectangular" width="100%" height={200} />
      ) : emptyState ? (
        <ListingTable.Container>
          <ListingTable.EmptyState {...emptyState} />
        </ListingTable.Container>
      ) : (
        <>
        {gatewayOffline && (
          <Alert
            severity="warning"
            onClose={() => setGatewayOffline(false)}
            sx={{ mb: 2 }}
          >
            The gateway is not connected to the control plane right now. The
            API key has been stored but will only work once the gateway
            reconnects.
          </Alert>
        )}
        <APIKeysManager
          keys={keys}
          isLoading={false}
          isError={isError}
          isCreating={isCreating}
          isRevoking={isRevoking}
          emptyDescription="Create an API key to authenticate requests to this agent."
          onCreate={handleCreate}
          onRevoke={handleRevoke}
        />
        </>
      )}
    </PageLayout>
  );
};

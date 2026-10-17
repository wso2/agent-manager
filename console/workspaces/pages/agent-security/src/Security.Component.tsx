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
import { AlertTriangle, KeyRound, Rocket, ShieldOff } from "@wso2/oxygen-ui-icons-react";
import {
  useCreateAgentAPIKey,
  useGetA2AProxy,
  useGetAgent,
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
import {
  buildConfigureTabHref,
  isExternalA2AAgent,
  PageLayout,
} from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";

const agentRoutes = absoluteRouteMap.children.org.children.projects.children.agents;

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
  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isExternalA2A = isExternalA2AAgent(agent);
  // Null while the environment is direct.
  const {
    data: proxy,
    isLoading: isLoadingProxy,
    isError: isProxyLookupFailed,
  } = useGetA2AProxy(
    { orgName: orgId, projName: projectId, agentName: agentId, envId },
    { enabled: isExternalA2A },
  );
  const { data: deployments, isLoading: isLoadingDeployments } =
    useListAgentDeployments({
      orgName: agent && !isExternalA2A ? orgId : undefined,
      projName: projectId,
      agentName: agentId,
    });

  const securityEnabled = envConfig?.enableApiKeySecurity ?? true;
  const oauthEnabled = envConfig?.enableOAuthSecurity ?? false;
  const currentDeployment = envId ? deployments?.[envId] : undefined;
  // A proxied external agent is served by the gateway, so its keys work without a deployment.
  const servedByGateway = isExternalA2A
    ? !!proxy
    : currentDeployment?.status === "active";
  const shouldLoadKeys =
    !isLoadingConfig &&
    !isLoadingDeployments &&
    servedByGateway &&
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
    isLoadingAgent ||
    isLoadingProxy ||
    isLoadingConfig ||
    isLoadingDeployments ||
    (shouldLoadKeys && isLoadingKeys);

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

  const a2aSettingsHref = orgId && projectId && agentId && envId
    ? buildConfigureTabHref(orgId, projectId, agentId, "a2a", envId)
    : undefined;
  const a2aSettingsAction = a2aSettingsHref ? (
    <Button variant="contained" component={Link} to={a2aSettingsHref}>
      Go to A2A settings
    </Button>
  ) : undefined;

  const emptyState = isExternalA2A && isProxyLookupFailed
    ? {
        illustration: <AlertTriangle size={48} />,
        title: "Could not load gateway routing",
        description: "Reload the page to try again.",
      }
    : !servedByGateway
    ? isExternalA2A
      ? {
          illustration: <Rocket size={48} />,
          title: "Agent is not routed through the gateway",
          description: "Turn on Route through gateway for this environment under Configure → A2A to manage its API keys.",
          action: a2aSettingsAction,
        }
      : {
          illustration: <Rocket size={48} />,
          title: "Agent is not deployed",
          description: "Deploy this agent to manage its API keys. Use the Deploy tab to get started.",
        }
    : !securityEnabled && oauthEnabled
    ? {
        illustration: <KeyRound size={48} />,
        title: "This agent uses OAuth",
        description: "Manage OAuth authentication from the configured identity provider.",
      }
    : !securityEnabled
    ? {
        illustration: <ShieldOff size={48} />,
        title: "Agent security is disabled",
        description: isExternalA2A
          ? "Enable API Key Security in the gateway security settings under Configure → A2A to manage API keys."
          : "Enable API Key Security from the deployment settings and redeploy to manage API keys.",
        action: isExternalA2A
          ? a2aSettingsAction
          : orgId && projectId && agentId && envId ? (
            <Button
              variant="contained"
              component={Link}
              to={generatePath(agentRoutes.children.environment.children.deploy.path, {
                orgId,
                projectId,
                agentId,
                envId,
              })}
            >
              Go to Deployment Settings
            </Button>
          ) : undefined,
      }
    : null;

  return (
    <PageLayout title="API Keys" disableIcon actions={<EnvironmentSelector />}>
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

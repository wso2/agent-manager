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
import { Alert, Button, Skeleton } from "@wso2/oxygen-ui";
import { KeyRound, Network, Rocket, ServerCrash, ShieldOff } from "@wso2/oxygen-ui-icons-react";
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
  isExternalAgent,
  PageLayout,
  StateNotice,
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
  const {
    data: agent,
    isLoading: isLoadingAgent,
    isError: isAgentLookupFailed,
  } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isExternal = isExternalAgent(agent);
  // Null while the environment is direct.
  const {
    data: proxy,
    isLoading: isLoadingProxy,
    isError: isProxyLookupFailed,
  } = useGetA2AProxy(
    { orgName: orgId, projName: projectId, agentName: agentId, envId },
    { enabled: isExternal },
  );
  const { data: deployments, isLoading: isLoadingDeployments } =
    useListAgentDeployments({
      orgName: agent && !isExternal ? orgId : undefined,
      projName: projectId,
      agentName: agentId,
    });

  const securityEnabled = envConfig?.enableApiKeySecurity ?? true;
  const oauthEnabled = envConfig?.enableOAuthSecurity ?? false;
  const currentDeployment = envId ? deployments?.[envId] : undefined;
  // A proxied external agent is served by the gateway, so its keys work without a deployment.
  const servedByGateway = isExternal
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

  const deployHref = orgId && projectId && agentId
    ? generatePath(agentRoutes.children.deployment.path, { orgId, projectId, agentId })
    : undefined;
  const deploySettingsAction = deployHref ? (
    <Button variant="contained" component={Link} to={deployHref}>
      Go to Deployment Settings
    </Button>
  ) : undefined;
  const reloadAction = (
    <Button variant="outlined" onClick={() => window.location.reload()}>
      Reload page
    </Button>
  );

  type Notice = Omit<React.ComponentProps<typeof StateNotice>, "contained" | "compact">;
  const notice: Notice | null = isAgentLookupFailed
    ? {
        icon: ServerCrash,
        tone: "error",
        title: "Could not load the agent",
        description: "The agent's details failed to load, so its keys can't be shown.",
        actions: reloadAction,
      }
    : isExternal && isProxyLookupFailed
    ? {
        icon: ServerCrash,
        tone: "error",
        title: "Could not load gateway routing",
        description: "Whether this agent is routed through the gateway failed to load.",
        actions: reloadAction,
      }
    : !servedByGateway
    ? isExternal
      ? {
          icon: Network,
          tone: "warning",
          title: "This agent is not routed through the gateway",
          description:
            "API keys are checked by the gateway. Turn on Route through gateway for this environment under Configure → A2A.",
          actions: a2aSettingsAction,
        }
      : {
          icon: Rocket,
          title: "Deploy your agent to manage its keys",
          description:
            "API keys are issued for a running agent, and this one isn't deployed in this environment yet.",
          steps: [
            { label: "Build", state: "done" },
            { label: "Deploy", state: "current" },
            { label: "Create keys", state: "todo" },
          ],
          actions: deployHref && (
            <Button variant="contained" component={Link} to={deployHref}>
              Go to Deploy
            </Button>
          ),
        }
    : !securityEnabled && oauthEnabled
    ? {
        icon: KeyRound,
        title: "This agent uses OAuth",
        description:
          "Callers authenticate with OAuth tokens from the configured identity provider, so there are no API keys to manage here.",
      }
    : !securityEnabled
    ? {
        icon: ShieldOff,
        tone: "warning",
        title: "API key security is off",
        description: isExternal
          ? "Enable API Key Security in the gateway security settings under Configure → A2A to manage API keys."
          : "Enable API Key Security in the deployment settings and redeploy to manage API keys.",
        actions: isExternal ? a2aSettingsAction : deploySettingsAction,
      }
    : null;

  return (
    <PageLayout title="API Keys" disableIcon actions={<EnvironmentSelector />}>
      {isLoading ? (
        <Skeleton variant="rectangular" width="100%" height={200} />
      ) : notice ? (
        <StateNotice {...notice} />
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

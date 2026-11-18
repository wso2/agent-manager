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

import { useEffect, useState } from "react";
import { Alert, Box, Button, Stack } from "@wso2/oxygen-ui";
import { generatePath, Navigate, useLocation, useNavigate, useParams } from "react-router-dom";
import { LoadErrorNotice, PageLayout } from "@agent-management-platform/views";
import { isGatewayActive, useGetGateway } from "@agent-management-platform/api-client";
import { absoluteRouteMap, isSelfHostedGateway } from "@agent-management-platform/types";
import { SelfHostedSetupSteps } from "./SelfHostedSetupSteps";
import { forgetNewGatewayToken, newGatewayToken } from "./newGatewayToken";

/** Router state the add page leaves for this page; never carries the token itself. */
export interface SetupGatewayState {
  /** The gateway was created, but minting its token failed. */
  tokenFailed?: boolean;
  /** Why minting failed, when known. */
  tokenError?: string;
}

/**
 * Setup page for a self-hosted gateway: the install steps, then a wait for the
 * gateway to connect. Reached from the add page right after creating the gateway,
 * which leaves the one-time token in memory for it (see newGatewayToken).
 */
export function SetupGateway() {
  const { gatewayId = "" } = useParams<{ gatewayId: string }>();
  // Keyed by gateway: moving between two gateways' setup pages starts afresh, so one
  // gateway's token is never shown for another.
  return <SetupGatewayPage key={gatewayId} gatewayId={gatewayId} />;
}

function SetupGatewayPage({ gatewayId }: { gatewayId: string }) {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const { state } = useLocation() as { state: SetupGatewayState | null };
  // Read once: the steps own the token from here (Reconfigure replaces it).
  const [token] = useState(() => newGatewayToken(gatewayId));
  // However the page is left (Done, Back, the sidebar), the token is forgotten: it may
  // be revoked afterwards, and a later visit must not offer it again.
  useEffect(() => () => forgetNewGatewayToken(gatewayId), [gatewayId]);

  // Polls until the gateway connects; step 5 of the steps shows the same status.
  const { data: gateway, error, refetch } = useGetGateway(
    { orgName: orgId, gatewayId },
    { pollUntilActive: true },
  );
  const connected = isGatewayActive(gateway);

  const listHref = generatePath(absoluteRouteMap.children.org.children.gateways.path, { orgId });
  const viewHref = generatePath(
    absoluteRouteMap.children.org.children.gateways.children.view.path,
    { orgId, gatewayId },
  );

  // Only a self-hosted gateway is set up by its owner.
  if (gateway && !isSelfHostedGateway(gateway)) {
    return <Navigate to={viewHref} replace />;
  }

  const finish = () => navigate(viewHref);

  // A gateway that cannot be loaded (not found, no access) has nothing to set up.
  if (error && !gateway) {
    return (
      <PageLayout
        title="Set up gateway"
        disableIcon
        backHref={listHref}
        backLabel="Back to gateways"
      >
        {/* Retrying stays on this page, which keeps the one-time token in memory. */}
        <LoadErrorNotice
          entityLabel="gateway"
          error={error}
          onRetry={() => void refetch()}
          backHref={listHref}
          backLabel="Back to gateways"
        />
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title={`Set up ${gateway?.displayName || gateway?.name || "gateway"}`}
      disableIcon
      backHref={listHref}
      backLabel="Back to gateways"
      isLoading={!gateway}
    >
      {/* Mounted only once the gateway has loaded: nothing can be reconfigured before
          its hosting is known, and PageLayout re-roots its children when loading ends,
          which would reset the steps' state. Keyed on the data, not isLoading: a first
          read paused offline is not loading, yet has no gateway. */}
      {gateway && (
      <Stack spacing={3}>
        {token && (
          <Alert severity="success">
            Gateway created. Run the steps below on your own infrastructure to connect it.
          </Alert>
        )}
        {!token && state?.tokenFailed && (
          <Alert severity="warning">
            Gateway created, but its token could not be generated.
            {state.tokenError ? ` ${state.tokenError}.` : ""} Use Reconfigure to generate one.
          </Alert>
        )}
        <SelfHostedSetupSteps orgName={orgId} gatewayId={gatewayId} initialToken={token} />
        <Box display="flex" justifyContent="flex-start" gap={1}>
          <Button variant="contained" onClick={finish} disabled={!connected}>
            Done
          </Button>
          {!connected && (
            <Button variant="text" onClick={finish}>
              Finish later
            </Button>
          )}
        </Box>
      </Stack>
      )}
    </PageLayout>
  );
}

export default SetupGateway;

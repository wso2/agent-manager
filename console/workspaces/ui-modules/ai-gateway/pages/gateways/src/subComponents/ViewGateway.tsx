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

import React from "react";
import {
  formatRelativeTime,
  GatewayTypeChip,
  getErrorMessage,
} from "@agent-management-platform/shared-component";
import {
  Alert,
  Button,
  Card,
  Chip,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Edit } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { isGatewayActive, useGetGateway } from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  isSelfHostedGateway,
} from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import { useGatewayFeatures } from "@agent-management-platform/feature-management";
import { GatewayIdentityProvidersCard } from "./GatewayIdentityProvidersCard";
import { SelfHostedGatewayTokensCard } from "./SelfHostedGatewayTokensCard";
import { SelfHostedSetupSteps } from "./SelfHostedSetupSteps";

export const ViewGateway: React.FC = () => {
  const { gatewayId, orgId } = useParams<{
    gatewayId: string;
    orgId: string;
  }>();
  const navigate = useNavigate();
  const { canWrite } = useGatewayFeatures();
  const {
    data: gateway,
    isLoading,
    error,
    refetch,
  } = useGetGateway({
    orgName: orgId,
    gatewayId,
  });

  const isSelfHosted = gateway !== undefined && isSelfHostedGateway(gateway);

  const displayName = gateway?.displayName ?? gateway?.name ?? gatewayId ?? "";
  const isActive = isGatewayActive(gateway);

  return (
    <>
      <PageLayout
        title={displayName}
        backHref={generatePath(
          absoluteRouteMap.children.org.children.gateways.path,
          { orgId: orgId ?? "" },
        )}
        backLabel="Back to gateways"
        description={
          gateway?.createdAt
            ? `Created ${formatRelativeTime(gateway.createdAt)}`
            : undefined
        }
        isLoading={isLoading}
        disableIcon
        actions={
          canWrite && gateway && isSelfHostedGateway(gateway) && (
          <Button
            variant="outlined"
            size="small"
            startIcon={<Edit size={16} />}
            disabled={!gateway}
            onClick={() =>
              navigate(
                generatePath(
                  absoluteRouteMap.children.org.children.gateways.children.edit.path,
                  { orgId: orgId ?? "", gatewayId: gatewayId ?? "" },
                ),
              )
            }
          >
            Edit gateway
          </Button>
          )
        }
        titleTail={
          gateway ? (
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              sx={{ ml: 1 }}
            >
              <Chip
                label={isActive ? "Active" : "Inactive"}
                size="small"
                variant="outlined"
                color={isActive ? "success" : "default"}
              />
              {gateway?.isCritical && (
                <Chip
                  label="Critical"
                  size="small"
                  variant="outlined"
                  color="error"
                />
              )}
              {gateway?.gatewayType && <GatewayTypeChip type={gateway.gatewayType} />}
            </Stack>
          ) : undefined
        }
      >
        {isLoading && (
          <Stack spacing={3}>
            <Card variant="outlined" sx={{ p: 3 }}>
              <Stack spacing={2}>
                <Skeleton variant="text" width={140} height={28} />
                <Skeleton variant="rounded" height={48} />
                <Skeleton variant="rounded" height={160} />
              </Stack>
            </Card>

            <Card variant="outlined" sx={{ p: 3 }}>
              <Stack spacing={2}>
                <Skeleton variant="text" width={160} height={24} />
                <Skeleton variant="text" width="60%" height={18} />
                <Stack spacing={1}>
                  <Skeleton variant="rounded" height={44} />
                  <Skeleton variant="rounded" height={44} />
                </Stack>
              </Stack>
            </Card>
          </Stack>
        )}
        {error ? (
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            sx={{ mb: 2 }}
          >
            Failed to load gateway. {getErrorMessage(error) || "Please try again."}
          </Alert>
        ) : null}

        {gateway && !error && (
          <Stack spacing={3}>
            {isSelfHosted && (
              <>
                {/* Setup is only needed until the gateway connects; once it is active,
                    tokens are rotated from the tokens card. */}
                {!isActive && (
                  <Card variant="outlined" sx={{ p: 3 }}>
                    <Stack spacing={2}>
                      <Typography variant="h6">Set up the gateway</Typography>
                      {/* The steps poll by UUID; this page may have been opened by
                          name, so it re-reads its own copy once they see the
                          gateway connect. */}
                      <SelfHostedSetupSteps
                        orgName={orgId ?? ""}
                        gatewayId={gateway.uuid}
                        onConnected={() => void refetch()}
                      />
                    </Stack>
                  </Card>
                )}
                <SelfHostedGatewayTokensCard
                  orgName={orgId ?? ""}
                  gatewayId={gateway.uuid}
                />
              </>
            )}
            <GatewayIdentityProvidersCard
              orgId={orgId ?? ""}
              gatewayId={gateway.uuid}
              gatewayName={gateway.displayName || gateway.name}
              environments={gateway.environments ?? []}
              selfHosted={isSelfHosted}
            />
          </Stack>
        )}
      </PageLayout>
    </>
  );
};

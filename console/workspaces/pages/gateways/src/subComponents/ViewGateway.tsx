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

import React, { useCallback, useState } from "react";
import {
  Alert,
  Card,
  Chip,
  Grid,
  Skeleton,
  Snackbar,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, useParams } from "react-router-dom";
import {
  useGetGateway,
  useListGatewayTokens,
  useRevokeGatewayToken,
  useRotateGatewayToken,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type GatewayTokenInfo,
} from "@agent-management-platform/types";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
import { ViewGatewayGetStarted } from "./ViewGatewayGetStarted";

export const ViewGateway: React.FC = () => {
  const { gatewayId, orgId } = useParams<{
    gatewayId: string;
    orgId: string;
  }>();
  const { addConfirmation } = useConfirmationDialog();
  const [registrationToken, setRegistrationToken] = useState<string | null>(
    null,
  );
  const [hasJustRegeneratedToken, setHasJustRegeneratedToken] = useState(false);
  const [copySnackbarOpen, setCopySnackbarOpen] = useState(false);
  const [copySnackbarMessage, setCopySnackbarMessage] = useState("");

  const {
    data: gateway,
    isLoading,
    error,
  } = useGetGateway({
    orgName: orgId,
    gatewayId,
  });

  const tokenParams = {
    orgName: orgId ?? "",
    gatewayId: gatewayId ?? "",
  };
  const { data: tokensData } = useListGatewayTokens(tokenParams);
  const { mutateAsync: rotateToken, isPending: isRotating } =
    useRotateGatewayToken();
  const { mutateAsync: revokeToken } = useRevokeGatewayToken();

  const handleCopy = useCallback((_text: string, label: string) => {
    setCopySnackbarMessage(`${label} copied to clipboard`);
    setCopySnackbarOpen(true);
  }, []);

  const isConfigured = (tokensData?.count ?? 0) > 0;

  const handleConfirmRegenerateToken = useCallback(async () => {
    if (!orgId || !gatewayId) return;
    try {
      const list: GatewayTokenInfo[] = tokensData?.list ?? [];
      await Promise.all(
        list
          .filter((t: GatewayTokenInfo) => t.status === "active")
          .map((t: GatewayTokenInfo) =>
            revokeToken({ orgName: orgId, gatewayId, tokenId: t.id }).catch(
              () => {},
            ),
          ),
      );
      const result = await rotateToken({ orgName: orgId, gatewayId });
      setRegistrationToken(result.token);
      setHasJustRegeneratedToken(true);
    } catch {
      // Error surfaced via mutation state if needed
    }
  }, [orgId, gatewayId, tokensData?.list, revokeToken, rotateToken]);

  const handleRegenerateToken = useCallback(() => {
    if (isConfigured) {
      addConfirmation({
        title: "Reconfigure gateway",
        description:
          "Regenerating the registration token will revoke the existing token for this gateway and disconnect the gateway from the control plane. Do you want to continue?",
        onConfirm: handleConfirmRegenerateToken,
        confirmButtonColor: "error",
        confirmButtonText: "Reconfigure",
      });
    } else {
      handleConfirmRegenerateToken();
    }
  }, [addConfirmation, isConfigured, handleConfirmRegenerateToken]);

  const displayName = gateway?.displayName ?? gateway?.name ?? gatewayId ?? "";
  const isActive =
    gateway?.status === "ACTIVE" ||
    (gateway as { isActive?: boolean } | undefined)?.isActive;

  return (
    <>
      <PageLayout
        title={displayName}
        backHref={generatePath(
          absoluteRouteMap.children.org.children.gateways.path,
          { orgId: orgId ?? "" },
        )}
        backLabel="Back to AI Gateways"
        description={
          gateway?.createdAt
            ? `Created ${formatDistanceToNow(new Date(gateway.createdAt), {
                addSuffix: true,
              })}`
            : undefined
        }
        isLoading={isLoading}
        disableIcon
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
            </Stack>
          ) : undefined
        }
      >
        {isLoading && (
          <Stack spacing={3}>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Skeleton variant="text" width="30%" height={14} />
                    <Skeleton variant="text" width="80%" height={20} />
                  </Stack>
                </Card>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Skeleton variant="text" width="35%" height={14} />
                    <Stack direction="row" spacing={0.5}>
                      <Skeleton variant="rounded" width={80} height={24} />
                      <Skeleton variant="rounded" width={80} height={24} />
                    </Stack>
                  </Stack>
                </Card>
              </Grid>
            </Grid>
            <Card variant="outlined" sx={{ p: 3 }}>
              <Stack spacing={2}>
                <Skeleton variant="text" width={120} height={24} />
                <Skeleton variant="rounded" height={48} />
                <Skeleton variant="rounded" height={120} />
              </Stack>
            </Card>
          </Stack>
        )}
        {error && (
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            sx={{ mb: 2 }}
          >
            Failed to load gateway.{" "}
            {error instanceof Error ? error.message : "Please try again."}
          </Alert>
        )}

        {gateway && !error && (
          <Stack spacing={3}>
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontWeight: 500 }}
                    >
                      Virtual Host
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: "monospace", wordBreak: "break-all" }}
                    >
                      {gateway.vhost}
                    </Typography>
                  </Stack>
                </Card>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                  <Stack spacing={0.5}>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ fontWeight: 500 }}
                    >
                      Environments
                    </Typography>
                    {gateway.environments && gateway.environments.length > 0 ? (
                      <Stack
                        direction="row"
                        spacing={0.5}
                        flexWrap="wrap"
                        useFlexGap
                      >
                        {gateway.environments.map((env) => (
                          <Chip
                            key={env.id}
                            label={env.displayName || env.name}
                            size="small"
                            variant="outlined"
                          />
                        ))}
                      </Stack>
                    ) : (
                      <Typography variant="body2" color="text.secondary">
                        —
                      </Typography>
                    )}
                  </Stack>
                </Card>
              </Grid>
            </Grid>

            <ViewGatewayGetStarted
              isConfigured={isConfigured}
              registrationToken={registrationToken}
              hasJustRegeneratedToken={hasJustRegeneratedToken}
              onRegenerateToken={handleRegenerateToken}
              isRegeneratingToken={isRotating}
              onCopy={handleCopy}
            />
          </Stack>
        )}
      </PageLayout>

      <Snackbar
        open={copySnackbarOpen}
        autoHideDuration={3000}
        onClose={() => setCopySnackbarOpen(false)}
        message={copySnackbarMessage}
      />
    </>
  );
};

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  Alert,
  Avatar,
  Chip,
  Grid,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Copy, DoorClosedLocked, KeyRound } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  copyToClipboard,
  GatewayTypeChip,
  getAvatarInitials,
  getErrorMessage,
  InfoCard,
} from "@agent-management-platform/shared-component";
import {
  useGetEnvironment,
  useListGateways,
  useListThunderInstances,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  isAgentIdentityEnabled,
  type GatewayResponse,
  type GatewayStatus,
} from "@agent-management-platform/types";
import { PageLayout, useSnackBar } from "@agent-management-platform/views";

const monoEllipsisSx = {
  fontFamily: "monospace",
  color: "text.secondary",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  display: "block",
} as const;

const STATUS_COLOR: Record<GatewayStatus, "success" | "warning" | "error" | "default"> = {
  ACTIVE: "success",
  INACTIVE: "default",
  PROVISIONING: "warning",
  ERROR: "error",
};

const GATEWAY_AVATAR_SX = {
  width: 28,
  height: 28,
  fontSize: 12,
  bgcolor: "primary.main",
  color: "primary.contrastText",
} as const;

export function EnvironmentViewPage() {
  const { orgId, envName } = useParams<{ orgId: string; envName: string }>();
  const navigate = useNavigate();
  const { pushSnackBar } = useSnackBar();

  const { data: env, isLoading: isLoadingEnv, error: envError } = useGetEnvironment({
    orgName: orgId,
    envName,
  });

  const { data: gatewaysData, isLoading: isLoadingGateways } = useListGateways(
    { orgName: orgId },
    { environment: envName },
  );

  const backHref = orgId
    ? generatePath(absoluteRouteMap.children.org.children.environments.path, { orgId })
    : "#";

  const gateways = gatewaysData?.gateways ?? [];

  // Thunder Id is part of the Agent ID feature set; skip the fetch entirely when
  // the flag is off so the hidden section costs nothing.
  const agentIdEnabled = isAgentIdentityEnabled();

  const { data: thunderInstancesData, isLoading: isLoadingProviders } =
    useListThunderInstances({ orgName: agentIdEnabled ? orgId : undefined });
  const thunderInstance = thunderInstancesData?.thunderInstances.find(
    (i) => i.envName === envName,
  );

  const handleCopy = (value: string, message: string) => {
    void copyToClipboard(value).then((succeeded) => {
      pushSnackBar(
        succeeded
          ? { message, type: "success" }
          : { message: "Failed to copy to clipboard", type: "error" },
      );
    });
  };

  const displayName = env?.displayName ?? env?.name ?? envName ?? "";

  return (
    <PageLayout
      title={displayName}
      backHref={backHref}
      backLabel="Back to Environments"
      description={
        env?.createdAt
          ? `Created ${formatDistanceToNow(new Date(env.createdAt), { addSuffix: true })}`
          : undefined
      }
      isLoading={isLoadingEnv}
      disableIcon
      titleTail={
        env ? (
          <Stack direction="row" alignItems="center" spacing={1}>
            <Chip
              label={env.isProduction ? "Production" : "Non-production"}
              size="small"
              variant="outlined"
              color={env.isProduction ? "primary" : "default"}
            />
            <Tooltip title="Data plane reference for this environment">
              <Chip label={env.dataplaneRef || "—"} size="small" variant="outlined" />
            </Tooltip>
          </Stack>
        ) : undefined
      }
    >
      {envError ? (
        <Alert severity="error" icon={<AlertTriangle size={18} />} sx={{ mb: 2 }}>
          {getErrorMessage(envError) || "Failed to load environment. Please try again."}
        </Alert>
      ) : null}

      {!isLoadingEnv && !env && !envError ? (
        <Alert severity="error" sx={{ mb: 2 }}>
          Environment &ldquo;{envName}&rdquo; not found.
        </Alert>
      ) : null}

      {isLoadingEnv && (
        <Stack spacing={3}>
          <Stack spacing={1.5}>
            <Skeleton variant="text" width={120} height={16} />
            <Grid container spacing={2}>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Skeleton variant="rounded" height={72} />
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Skeleton variant="rounded" height={72} />
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Skeleton variant="rounded" height={72} />
              </Grid>
            </Grid>
          </Stack>

          <Stack spacing={1.5}>
            <Skeleton variant="text" width={90} height={16} />
            <Stack spacing={1}>
              <Skeleton variant="rounded" height={56} />
              <Skeleton variant="rounded" height={56} />
            </Stack>
          </Stack>

          <Stack spacing={1.5}>
            <Skeleton variant="text" width={140} height={16} />
            <Skeleton variant="rounded" height={56} />
          </Stack>
        </Stack>
      )}

      {env && !envError && (
        <Stack spacing={3}>
          {env.dnsPrefix && (
            <Stack spacing={1.5}>
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                  <InfoCard label="DNS Prefix" value={env.dnsPrefix} />
                </Grid>
              </Grid>
            </Stack>
          )}

          <Stack spacing={1.5}>
            <Typography variant="overline" color="text.secondary">
              Gateways
            </Typography>
            {isLoadingGateways ? (
              <Stack spacing={1}>
                {Array.from({ length: 3 }).map((_, i) => (
                  <Skeleton key={i} variant="rounded" height={56} />
                ))}
              </Stack>
            ) : gateways.length === 0 ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<DoorClosedLocked size={56} />}
                  title="No gateways in this environment"
                  description="Assign a gateway to this environment to see it here."
                />
              </ListingTable.Container>
            ) : (
              <ListingTable.Container>
                <ListingTable variant="table">
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell>Gateway</ListingTable.Cell>
                      <ListingTable.Cell>Type</ListingTable.Cell>
                      <ListingTable.Cell>Virtual Host</ListingTable.Cell>
                      <ListingTable.Cell align="center">Status</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {gateways.map((gw: GatewayResponse) => (
                      <ListingTable.Row
                        key={gw.uuid}
                        variant="table"
                        hover
                        clickable
                        onClick={() =>
                          navigate(
                            generatePath(
                              absoluteRouteMap.children.org.children.gateways.children.view.path,
                              { orgId: orgId ?? "", gatewayId: gw.uuid },
                            ),
                          )
                        }
                      >
                        <ListingTable.Cell>
                          <ListingTable.CellIcon
                            icon={
                              <Avatar sx={GATEWAY_AVATAR_SX}>
                                {getAvatarInitials(gw.displayName ?? gw.name, {
                                  fallback: "G",
                                  maxChars: 1,
                                })}
                              </Avatar>
                            }
                            primary={gw.displayName ?? gw.name}
                          />
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <GatewayTypeChip type={gw.gatewayType} />
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                            {gw.vhost}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell align="center">
                          <Chip
                            label={gw.status}
                            size="small"
                            color={STATUS_COLOR[gw.status]}
                            variant="outlined"
                          />
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    ))}
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            )}
          </Stack>

          {agentIdEnabled && (
          <Stack spacing={1.5}>
            <Typography variant="overline" color="text.secondary">
              Identity Providers
            </Typography>
            {isLoadingProviders ? (
              <Stack spacing={1}>
                <Skeleton variant="rounded" height={56} />
              </Stack>
            ) : !thunderInstance ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<KeyRound size={56} />}
                  title="No Thunder Id in this environment"
                  description="Each environment automatically gets a Thunder Id."
                />
              </ListingTable.Container>
            ) : (
              <ListingTable.Container>
                <ListingTable variant="table">
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell width="240px">Identity Provider</ListingTable.Cell>
                      <ListingTable.Cell>Issuer</ListingTable.Cell>
                      <ListingTable.Cell>Token Endpoint</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    <ListingTable.Row
                      variant="table"
                      hover
                      clickable
                      onClick={() =>
                        navigate(
                          generatePath(
                            absoluteRouteMap.children.org.children.environments.children.view
                              .children.identityProvider.path,
                            { orgId: orgId ?? "", envName: thunderInstance.envName },
                          ),
                        )
                      }
                    >
                      <ListingTable.Cell>
                        <ListingTable.CellIcon
                          icon={
                            <Avatar
                              sx={{
                                width: 28,
                                height: 28,
                                bgcolor: "primary.main",
                                color: "primary.contrastText",
                              }}
                            >
                              <KeyRound size={16} />
                            </Avatar>
                          }
                          primary="Thunder Id"
                          secondary="System identity provider"
                        />
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Typography variant="caption" sx={{ ...monoEllipsisSx, maxWidth: 280 }}>
                          {thunderInstance.issuerUrl}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell>
                        <Stack direction="row" alignItems="center" spacing={1}>
                          <Typography variant="caption" sx={{ ...monoEllipsisSx, maxWidth: 320 }}>
                            {thunderInstance.tokenUrl}
                          </Typography>
                          <Tooltip title="Copy token endpoint">
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleCopy(
                                  thunderInstance.tokenUrl,
                                  "Token endpoint copied to clipboard",
                                );
                              }}
                            >
                              <Copy size={14} />
                            </IconButton>
                          </Tooltip>
                        </Stack>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            )}
          </Stack>
          )}
        </Stack>
      )}
    </PageLayout>
  );
}

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

import { useMemo, useState } from "react";
import {
  Alert,
  Avatar,
  Button,
  Card,
  CardContent,
  Chip,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, KeyRound, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { useListIdentityProviders } from "@agent-management-platform/api-client";
import type {
  GatewayEnvironmentResponse,
  IdentityProvider,
} from "@agent-management-platform/types";
import {
  ManageIdentityProviderDialog,
  type ManageIdentityProviderMode,
} from "./ManageIdentityProviderDialog";
import { useGatewayFeatures } from "@agent-management-platform/feature-management";

interface GatewayIdentityProvidersCardProps {
  orgId: string;
  gatewayId: string;
  gatewayName: string;
  environments: GatewayEnvironmentResponse[];
}

export function GatewayIdentityProvidersCard({
  orgId,
  gatewayId,
  gatewayName,
  environments,
}: GatewayIdentityProvidersCardProps) {
  const [hoveredKey, setHoveredKey] = useState<string | null>(null);
  const { canWrite } = useGatewayFeatures();
  const [dialogMode, setDialogMode] =
    useState<ManageIdentityProviderMode>("upsert");
  const [dialogProvider, setDialogProvider] = useState<IdentityProvider | null>(
    null,
  );
  const [dialogOpen, setDialogOpen] = useState(false);

  const {
    data: providersResp,
    isLoading,
    error,
  } = useListIdentityProviders({ orgName: orgId });

  // The org-wide list returns every provider with its gateway attached; scope to
  // the gateway being viewed. A provider may appear once per environment its
  // gateway maps to, so the environment column disambiguates rows.
  const providers = useMemo(
    () => (providersResp?.list ?? []).filter((p) => p.gatewayId === gatewayId),
    [providersResp, gatewayId],
  );

  const openAdd = () => {
    setDialogMode("upsert");
    setDialogProvider(null);
    setDialogOpen(true);
  };

  const openDelete = (provider: IdentityProvider) => {
    setDialogMode("delete");
    setDialogProvider(provider);
    setDialogOpen(true);
  };

  const rowKey = (p: IdentityProvider) =>
    `${p.environmentName ?? ""}:${p.name}`;

  const lockedGateway = useMemo(
    () => ({ uuid: gatewayId, name: gatewayName, environments }),
    [gatewayId, gatewayName, environments],
  );

  const addButton = canWrite ? (
    <Button
      variant="contained"
      color="primary"
      size="small"
      startIcon={<Plus size={16} />}
      onClick={openAdd}
    >
      Add identity provider
    </Button>
  ) : undefined;

  const dialog = canWrite && (
    <ManageIdentityProviderDialog
      open={dialogOpen}
      onClose={() => setDialogOpen(false)}
      orgId={orgId}
      mode={dialogMode}
      provider={dialogProvider}
      lockedGateway={lockedGateway}
    />
  );

  const renderBody = () => {
    if (error) {
      return (
        <Alert severity="error" icon={<AlertTriangle size={18} />}>
          {error instanceof Error
            ? error.message
            : "Failed to load identity providers. Please try again."}
        </Alert>
      );
    }

    if (isLoading) {
      return (
        <Stack spacing={1}>
          {Array.from({ length: 2 }).map((_, i) => (
            <Stack
              key={i}
              direction="row"
              alignItems="center"
              spacing={2}
              sx={{
                px: 2,
                py: 1.5,
                borderRadius: 1,
                border: "1px solid",
                borderColor: "divider",
              }}
            >
              <Skeleton variant="circular" width={28} height={28} sx={{ flexShrink: 0 }} />
              <Skeleton variant="text" width={160} height={20} />
              <Skeleton
                variant="rounded"
                width={72}
                height={24}
                sx={{ flexShrink: 0 }}
              />
              <Skeleton variant="text" sx={{ flex: 1 }} height={18} />
            </Stack>
          ))}
        </Stack>
      );
    }

    if (!providers.length) {
      return (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<KeyRound size={48} />}
            title="No identity providers configured"
            description="Add an identity provider (token issuer) so agents on this gateway can authenticate callers with OAuth."
            action={addButton}
          />
        </ListingTable.Container>
      );
    }

    return (
      <ListingTable variant="card">
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell width="220px">Name</ListingTable.Cell>
            <ListingTable.Cell width="110px">Type</ListingTable.Cell>
            <ListingTable.Cell>Issuer</ListingTable.Cell>
            <ListingTable.Cell width="160px" align="right">
              Environment
            </ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {providers.map((provider) => {
            const key = rowKey(provider);
            const isSystem = provider.type === "system";
            return (
              <ListingTable.Row
                key={key}
                variant="card"
                hover
                onMouseEnter={() => setHoveredKey(key)}
                onMouseLeave={() => setHoveredKey(null)}
                onFocus={() => setHoveredKey(key)}
                onBlur={() => setHoveredKey(null)}
              >
                <ListingTable.Cell>
                  <ListingTable.CellIcon
                    icon={
                      <Avatar
                        sx={{
                          width: 28,
                          height: 28,
                          bgcolor: isSystem ? "action.selected" : "primary.main",
                          color: isSystem ? "text.secondary" : "primary.contrastText",
                        }}
                      >
                        <KeyRound size={16} />
                      </Avatar>
                    }
                    primary={provider.name}
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Chip
                    label={isSystem ? "System" : "Custom"}
                    size="small"
                    color={isSystem ? "default" : "primary"}
                    variant="outlined"
                  />
                </ListingTable.Cell>
                <ListingTable.Cell>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    sx={{ wordBreak: "break-all" }}
                  >
                    {provider.issuer || "—"}
                  </Typography>
                </ListingTable.Cell>
                <ListingTable.Cell align="right">
                  {/* Fixed-height container so swapping the environment label for
                      the delete icon on hover never changes the row height. */}
                  <Stack
                    direction="row"
                    alignItems="center"
                    spacing={1}
                    justifyContent="flex-end"
                    sx={{ minHeight: 32 }}
                  >
                    {canWrite && hoveredKey === key && !isSystem ? (
                      <Tooltip title="Remove identity provider">
                        <IconButton
                          color="error"
                          size="small"
                          onClick={() => openDelete(provider)}
                        >
                          <Trash size={16} />
                        </IconButton>
                      </Tooltip>
                    ) : (
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        noWrap
                        title={provider.environmentName || undefined}
                      >
                        {provider.environmentName || "—"}
                      </Typography>
                    )}
                  </Stack>
                </ListingTable.Cell>
              </ListingTable.Row>
            );
          })}
        </ListingTable.Body>
      </ListingTable>
    );
  };

  return (
    <Card variant="outlined">
      <CardContent sx={{ p: 3 }}>
        <Stack spacing={2}>
          <Stack
            direction="row"
            justifyContent="space-between"
            alignItems="center"
          >
            <Stack spacing={0.5}>
              <Typography variant="h6">Token Issuers</Typography>
              <Typography variant="body2" color="text.secondary">
                Token issuers configured on this gateway. Secure agent endpoints
                with OAuth.
              </Typography>
            </Stack>
            {!isLoading && !error && providers.length > 0 ? (addButton ?? null) : null}
          </Stack>
          {renderBody()}
        </Stack>
      </CardContent>
      {dialog}
    </Card>
  );
}

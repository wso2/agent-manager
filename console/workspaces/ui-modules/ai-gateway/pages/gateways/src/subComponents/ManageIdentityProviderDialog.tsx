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

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  FormControl,
  FormControlLabel,
  FormLabel,
  IconButton,
  MenuItem,
  Select,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Copy, KeyRound, Trash } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import {
  useDeleteIdentityProvider,
  useDiscoverOidc,
  useListEnvironments,
  useListGateways,
  useUpsertIdentityProvider,
} from "@agent-management-platform/api-client";
import {
  isManagedGateway,
  type GatewayEnvironmentResponse,
  type IdentityProvider,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import {
  useConfirmIfUnsaved,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import {
  buildKeyManagerConfig,
  type ManageIdentityProviderMode,
} from "./gatewayKeyManagerConfig";
import { useFeatureFlag } from "@agent-management-platform/feature-management";

export type { ManageIdentityProviderMode };

interface ManageIdentityProviderDialogProps {
  open: boolean;
  onClose: () => void;
  orgId: string;
  mode: ManageIdentityProviderMode;
  /** For delete, the provider row to remove (name + gateway + environment prefilled). */
  provider?: IdentityProvider | null;
  /**
   * When opened from a specific gateway, the gateway is fixed: the gateway picker
   * is hidden and the environment choices are constrained to this gateway's
   * environments (auto-selected when there is only one).
   */
  lockedGateway?: {
    uuid: string;
    name: string;
    environments: GatewayEnvironmentResponse[];
  } | null;
}

export function ManageIdentityProviderDialog({
  open,
  onClose,
  orgId,
  mode,
  provider,
  lockedGateway,
}: ManageIdentityProviderDialogProps) {
  const isDelete = mode === "delete";
  const isGatewayLocked = !!lockedGateway;

  const [envName, setEnvName] = useState("");
  const [gatewayId, setGatewayId] = useState("");
  const [name, setName] = useState("");
  const [issuer, setIssuer] = useState("");
  const [jwksUri, setJwksUri] = useState("");
  const [skipTlsVerify, setSkipTlsVerify] = useState(false);

  // Optional OIDC discovery: paste an issuer / .well-known URL to auto-fill the
  // issuer and JWKS URI below. Manual entry remains the primary path.
  const [discoveryUrl, setDiscoveryUrl] = useState("");
  const [discoveryError, setDiscoveryError] = useState<string | null>(null);
  const discover = useDiscoverOidc({ orgName: orgId });

  const [copied, setCopied] = useState(false);

  // Managed mode: the server patches the gateway runtime config directly. When
  // off (self-hosted default), the operator edits the gateway's config.toml;
  // either way the submit records the provider in Agent Manager.
  const managedMode = useFeatureFlag("enableIdentityProviderManagedMode");
  const upsertIdp = useUpsertIdentityProvider();
  const deleteIdp = useDeleteIdentityProvider();
  const isSubmitting = upsertIdp.isPending || deleteIdp.isPending;

  const canSubmit = isDelete
    ? !!gatewayId && !!name
    : !!gatewayId && !!name.trim() && !!issuer.trim() && !!jwksUri.trim();

  const handleSubmit = useCallback(async () => {
    try {
      if (isDelete) {
        await deleteIdp.mutateAsync({ orgName: orgId, gatewayId, name });
      } else {
        await upsertIdp.mutateAsync({
          params: { orgName: orgId, gatewayId, name: name.trim() },
          body: {
            issuer: issuer.trim(),
            jwksUri: jwksUri.trim(),
            skipTlsVerify,
          },
        });
      }
      onClose();
    } catch {
      // Failure is surfaced via the mutation's error notification; keep the dialog open.
    }
  }, [
    isDelete,
    deleteIdp,
    upsertIdp,
    orgId,
    gatewayId,
    name,
    issuer,
    jwksUri,
    skipTlsVerify,
    onClose,
  ]);

  const { data: environments } = useListEnvironments({ orgName: orgId });
  // The selected environment's managed gateways populate the gateway picker (only used
  // when no gateway is locked in): a self-hosted gateway has no identity providers here.
  const { data: gatewaysResp } = useListGateways(
    { orgName: orgId },
    envName ? { environment: envName } : undefined,
  );
  const gateways = useMemo(
    () => (gatewaysResp?.gateways ?? []).filter(isManagedGateway),
    [gatewaysResp],
  );

  // Environment choices: scoped to the locked gateway when present, otherwise
  // every environment in the org.
  const envOptions = useMemo<
    Array<{ name: string; displayName?: string }>
  >(
    () => (lockedGateway ? lockedGateway.environments : (environments ?? [])),
    [lockedGateway, environments],
  );

  // Reset form when the dialog opens, seeding from the provider for delete.
  useEffect(() => {
    if (!open) return;
    setCopied(false);
    setDiscoveryUrl("");
    setDiscoveryError(null);
    if (isDelete && provider) {
      setEnvName(provider.environmentName ?? "");
      setGatewayId(provider.gatewayId ?? "");
      setName(provider.name);
      setIssuer(provider.issuer ?? "");
      setJwksUri(provider.jwksUri ?? "");
      setSkipTlsVerify(provider.skipTlsVerify ?? false);
    } else {
      // Lock the gateway and auto-select its environment when there is only one.
      setEnvName(
        lockedGateway && lockedGateway.environments.length === 1
          ? lockedGateway.environments[0].name
          : "",
      );
      setGatewayId(lockedGateway?.uuid ?? "");
      setName("");
      setIssuer("");
      setJwksUri("");
      setSkipTlsVerify(false);
    }
  }, [open, isDelete, provider, lockedGateway]);

  // Delete mode is read-only; in upsert mode only user-entered values count.
  const isDirty =
    open &&
    !isDelete &&
    (!!name.trim() ||
      !!issuer.trim() ||
      !!jwksUri.trim() ||
      skipTlsVerify ||
      !!discoveryUrl.trim() ||
      (!isGatewayLocked && !!envName));
  useUnsavedChangesGuard(isDirty);
  const confirmIfUnsaved = useConfirmIfUnsaved();
  // Backdrop and header X discard edits, so confirm first; Cancel and the
  // post-save close stay direct.
  const handleGuardedClose = () => confirmIfUnsaved(onClose, isDirty);

  const handleDiscover = useCallback(async () => {
    const url = discoveryUrl.trim();
    if (!url) return;
    setDiscoveryError(null);
    try {
      const result = await discover.mutateAsync({ url });
      setIssuer(result.issuer);
      setJwksUri(result.jwksUri);
    } catch {
      setDiscoveryError(
        "Could not discover OIDC configuration from that URL. Check the URL or enter the issuer and JWKS URI manually.",
      );
    }
  }, [discoveryUrl, discover]);

  const keyManagerConfig = useMemo(
    () => buildKeyManagerConfig({ name, issuer, jwksUri, skipTlsVerify }),
    [name, issuer, jwksUri, skipTlsVerify],
  );

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(keyManagerConfig);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // silently fail
    }
  }, [keyManagerConfig]);

  const environmentField = (
    <FormControl fullWidth disabled={isDelete}>
      <FormLabel>Environment</FormLabel>
      <Select
        size="small"
        value={envName}
        displayEmpty
        onChange={(e) => setEnvName(e.target.value as string)}
        renderValue={(v) => {
          if (!v) return "Select an environment";
          const env = envOptions.find((en) => en.name === v);
          return env?.displayName || env?.name || (v as string);
        }}
      >
        {envOptions.map((env) => (
          <MenuItem key={env.name} value={env.name}>
            {env.displayName || env.name}
          </MenuItem>
        ))}
      </Select>
    </FormControl>
  );

  const gatewayField = (
    <FormControl fullWidth disabled={isDelete || (!isGatewayLocked && !envName)}>
      <FormLabel>Gateway</FormLabel>
      {isGatewayLocked ? (
        <TextField
          size="small"
          fullWidth
          value={lockedGateway?.name ?? ""}
          slotProps={{ input: { readOnly: true } }}
          disabled
        />
      ) : (
        <Select
          size="small"
          value={gatewayId}
          displayEmpty
          onChange={(e) => setGatewayId(e.target.value as string)}
          renderValue={(v) => {
            if (!v) return "Select a gateway";
            const gw = gateways.find((g) => g.uuid === v);
            return gw?.displayName || gw?.name || (v as string);
          }}
        >
          {gateways.map((gw) => (
            <MenuItem key={gw.uuid} value={gw.uuid}>
              {gw.displayName || gw.name}
            </MenuItem>
          ))}
        </Select>
      )}
    </FormControl>
  );

  return (
    <DrawerWrapper open={open} onClose={handleGuardedClose}>
      <DrawerHeader
        icon={isDelete ? <Trash size={24} /> : <KeyRound size={24} />}
        title={isDelete ? "Remove Identity Provider" : "Add Identity Provider"}
        onClose={handleGuardedClose}
      />
      <DrawerContent>
        <Stack spacing={3}>
          <Typography variant="body2" color="text.secondary">
            {managedMode ? (
              <>
                Identity providers are owned by the gateway. Agent Manager patches the gateway
                configuration and applies the change for you.
              </>
            ) : (
              <>
                Identity providers are owned by the gateway: its <code>configs/config.toml</code>{" "}
                lists the token issuers it accepts. Update that file and restart the gateway, then{" "}
                {isDelete ? "remove" : "add"} the provider here so Agent Manager lists it.
              </>
            )}
          </Typography>

          <Stack spacing={2}>
            {/* Gateway comes first when it is locked in (added from a gateway);
                otherwise Environment leads, because the gateway picker is
                filtered by the selected environment. */}
            {isGatewayLocked ? (
              <>
                {gatewayField}
                {environmentField}
              </>
            ) : (
              <>
                {environmentField}
                {gatewayField}
              </>
            )}

            <FormControl fullWidth disabled={isDelete}>
              <FormLabel>Name</FormLabel>
              <TextField
                slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                size="small"
                fullWidth
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="e.g., AcmeIdP"
                disabled={isDelete}
              />
            </FormControl>

            {!isDelete && (
              <>
                <FormControl fullWidth>
                  <FormLabel>Discover from URL (optional)</FormLabel>
                  <Stack direction="row" spacing={1}>
                    <TextField
                      slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.URL } }}
                      size="small"
                      fullWidth
                      value={discoveryUrl}
                      onChange={(e) => setDiscoveryUrl(e.target.value)}
                      placeholder="https://idp.example.com (or .well-known URL)"
                    />
                    <Button
                      variant="outlined"
                      onClick={handleDiscover}
                      disabled={!discoveryUrl.trim() || discover.isPending}
                    >
                      {discover.isPending ? "Fetching…" : "Fetch"}
                    </Button>
                  </Stack>
                  <Typography variant="caption" color="text.secondary">
                    Auto-fills the issuer and JWKS URI from the provider&apos;s OpenID
                    configuration.
                  </Typography>
                </FormControl>

                {discoveryError && <Alert severity="warning">{discoveryError}</Alert>}

                <FormControl fullWidth>
                  <FormLabel>Issuer</FormLabel>
                  <TextField
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.URL } }}
                    size="small"
                    fullWidth
                    value={issuer}
                    onChange={(e) => setIssuer(e.target.value)}
                    placeholder="https://idp.example.com"
                  />
                </FormControl>

                <FormControl fullWidth>
                  <FormLabel>JWKS URI</FormLabel>
                  <TextField
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.URL } }}
                    size="small"
                    fullWidth
                    value={jwksUri}
                    onChange={(e) => setJwksUri(e.target.value)}
                    placeholder="https://idp.example.com/oauth2/jwks"
                  />
                </FormControl>

                <FormControlLabel
                  control={
                    <Checkbox
                      checked={skipTlsVerify}
                      onChange={(e) => setSkipTlsVerify(e.target.checked)}
                    />
                  }
                  label="Skip TLS verification when fetching JWKS (trusted internal only)"
                />
              </>
            )}
          </Stack>

          {!managedMode && (
            <Stack spacing={1}>
              <Typography variant="body2">
                {isDelete
                  ? "Remove this entry from the gateway's configs/config.toml, then restart the gateway:"
                  : "Add this entry to the gateway's configs/config.toml, then restart the gateway:"}
              </Typography>
              <Box sx={{ position: "relative", bgcolor: "grey.900", borderRadius: 1 }}>
                <Box sx={{ position: "absolute", top: 6, right: 6 }}>
                  <Tooltip title={copied ? "Copied!" : "Copy"}>
                    <IconButton
                      size="small"
                      onClick={handleCopy}
                      sx={{ color: copied ? "success.light" : "grey.400" }}
                    >
                      <Copy size={16} />
                    </IconButton>
                  </Tooltip>
                </Box>
                {/* Scrolls on its own so the copy icon stays put. */}
                <Box
                  sx={{
                    p: 2,
                    pr: 6,
                    fontFamily: "monospace",
                    fontSize: "0.8125rem",
                    color: "grey.100",
                    whiteSpace: "pre",
                    overflowX: "auto",
                  }}
                >
                  {keyManagerConfig}
                </Box>
              </Box>
            </Stack>
          )}

          <Box display="flex" justifyContent="flex-end" gap={1}>
            <Button variant="outlined" color="inherit" onClick={onClose}>
              Cancel
            </Button>
            <Button
              variant="contained"
              color={isDelete ? "error" : "primary"}
              onClick={handleSubmit}
              disabled={!canSubmit || isSubmitting}
            >
              {isSubmitting
                ? isDelete
                  ? "Removing…"
                  : "Saving…"
                : isDelete
                  ? "Remove"
                  : "Add"}
            </Button>
          </Box>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

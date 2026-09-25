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
import { Copy, Eye, EyeOff, KeyRound, Trash } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  useDeleteIdentityProvider,
  useDiscoverOidc,
  useListEnvironments,
  useListGateways,
  useUpsertIdentityProvider,
} from "@agent-management-platform/api-client";
import {
  globalConfig,
  type GatewayEnvironmentResponse,
  type IdentityProvider,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import {
  getAgentManagerUrl,
  getAmpVersionHelm,
  getRawScriptUrl,
} from "@agent-management-platform/shared-component";

const SCRIPT_NAME = "manage-identity-provider.sh";
const TOKEN_MASK = "•••••••••••••••";

export type ManageIdentityProviderMode = "upsert" | "delete";

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

export interface ScriptInputs {
  orgId: string;
  envName: string;
  gatewayId: string;
  name: string;
  issuer: string;
  jwksUri: string;
  skipTlsVerify: boolean;
  mode: ManageIdentityProviderMode;
  token: string;
}

/** Quote a value as one POSIX shell word without evaluating metacharacters. */
export function quoteShellWord(value: string): string {
  return `'${value.replace(/'/g, `'"'"'`)}'`;
}

export function buildScript(i: ScriptInputs): string {
  // The dev-container quickstart and a real cluster both install the gateway
  // extension from the same OCI chart, so one command works for both: curl the
  // release-pinned script and upgrade the OCI chart at the deployed version.
  const actionEnvLines =
    i.mode === "delete"
      ? ["    ACTION=delete \\"]
      : [
          `    IDP_ISSUER=${quoteShellWord(i.issuer || "<issuer>")} \\`,
          `    IDP_JWKS_URI=${quoteShellWord(i.jwksUri || "<jwks-uri>")} \\`,
          ...(i.skipTlsVerify ? ["    IDP_SKIP_TLS_VERIFY=true \\"] : []),
        ];
  const lines = [
    `curl -fsSL ${quoteShellWord(getRawScriptUrl(SCRIPT_NAME))} \\`,
    `  | ORG_NAME=${quoteShellWord(i.orgId || "<org>")} \\`,
    `    ENV_NAME=${quoteShellWord(i.envName || "<env-name>")} \\`,
    `    GATEWAY_ID=${quoteShellWord(i.gatewayId || "<gateway-id>")} \\`,
    `    AGENT_MANAGER_TOKEN=${quoteShellWord(i.token)} \\`,
    `    AGENT_MANAGER_URL=${quoteShellWord(getAgentManagerUrl())} \\`,
    `    CHART_VERSION=${quoteShellWord(getAmpVersionHelm())} \\`,
    `    IDP_NAME=${quoteShellWord(i.name || "<identity-provider-name>")} \\`,
    ...actionEnvLines,
    "    bash",
  ];
  return lines.join("\n");
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
  const { getToken } = useAuthHooks();

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

  const [showToken, setShowToken] = useState(false);
  const [resolvedToken, setResolvedToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  // Managed mode: when enabled, the server patches the gateway runtime config
  // directly. When off (self-hosted default), the dialog renders the script
  // snippet exactly as before.
  const managedMode = globalConfig.featureFlags?.enableIdentityProviderManagedMode === true;
  const upsertIdp = useUpsertIdentityProvider();
  const deleteIdp = useDeleteIdentityProvider();
  const isSubmitting = upsertIdp.isPending || deleteIdp.isPending;

  const canSubmit = isDelete
    ? !!gatewayId && !!name
    : !!gatewayId && !!name.trim() && !!issuer.trim() && !!jwksUri.trim();

  const handleManagedSubmit = useCallback(async () => {
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
  // Gateways belonging to the selected environment populate the gateway picker
  // (only used when no gateway is locked in).
  const { data: gatewaysResp } = useListGateways(
    { orgName: orgId },
    envName ? { environment: envName } : undefined,
  );
  const gateways = useMemo(() => gatewaysResp?.gateways ?? [], [gatewaysResp]);

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
    setShowToken(false);
    setResolvedToken(null);
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

  // Clear the gateway selection when the environment changes (upsert flow only —
  // a locked gateway stays fixed regardless of environment).
  useEffect(() => {
    if (isDelete || lockedGateway) return;
    setGatewayId("");
  }, [envName, isDelete, lockedGateway]);

  const handleToggleToken = useCallback(async () => {
    if (showToken) {
      setShowToken(false);
      setResolvedToken(null);
      return;
    }
    try {
      const token = await getToken();
      setResolvedToken(token);
      setShowToken(true);
    } catch {
      // silently fail
    }
  }, [showToken, getToken]);

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

  const scriptInputs = useCallback(
    (token: string): ScriptInputs => ({
      orgId,
      envName,
      gatewayId,
      name,
      issuer,
      jwksUri,
      skipTlsVerify,
      mode,
      token,
    }),
    [orgId, envName, gatewayId, name, issuer, jwksUri, skipTlsVerify, mode],
  );

  const handleCopy = useCallback(async () => {
    try {
      const token = resolvedToken ?? (await getToken());
      await navigator.clipboard.writeText(buildScript(scriptInputs(token)));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // silently fail
    }
  }, [resolvedToken, getToken, scriptInputs]);

  const displayScript = useMemo(
    () => buildScript(scriptInputs(showToken && resolvedToken ? resolvedToken : TOKEN_MASK)),
    [scriptInputs, showToken, resolvedToken],
  );

  const environmentField = (
    <FormControl fullWidth disabled={isDelete}>
      <FormLabel required>Environment</FormLabel>
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
      <FormLabel required>Gateway</FormLabel>
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
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={isDelete ? <Trash size={24} /> : <KeyRound size={24} />}
        title={isDelete ? "Remove Identity Provider" : "Add Identity Provider"}
        onClose={onClose}
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
                Identity providers are owned by the gateway. This command patches the gateway
                configuration and syncs Agent Manager. Run it in a terminal with{" "}
                <code>kubectl</code>, <code>helm</code>, and <code>jq</code> configured against
                your cluster — for the quickstart, run it inside the dev container shell.
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
              <FormLabel required>Name</FormLabel>
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
                  <FormLabel required>Issuer</FormLabel>
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
                  <FormLabel required>JWKS URI</FormLabel>
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
            <Typography variant="body2">Run this command:</Typography>
            <Box
              sx={{
                position: "relative",
                bgcolor: "grey.900",
                borderRadius: 1,
                p: 2,
                pr: 8,
                fontFamily: "monospace",
                fontSize: "0.8125rem",
                color: "grey.100",
                whiteSpace: "pre",
                overflowX: "auto",
              }}
            >
              <Box sx={{ position: "absolute", top: 6, right: 6, display: "flex", gap: 0.5 }}>
                <Tooltip title={showToken ? "Hide token" : "Show token"}>
                  <IconButton size="small" onClick={handleToggleToken} sx={{ color: "grey.400" }}>
                    {showToken ? <EyeOff size={16} /> : <Eye size={16} />}
                  </IconButton>
                </Tooltip>
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
              {displayScript}
            </Box>
            <Typography variant="caption" color="text.secondary">
              Your access token will be substituted when you copy.
            </Typography>
          </Stack>
          )}

          {!managedMode && (
            <Alert severity="info">
              Once the script completes, the list reflects the change. The script is idempotent —
              safe to re-run.
            </Alert>
          )}

          <Box display="flex" justifyContent="flex-end" gap={1}>
            <Button variant="outlined" color="inherit" onClick={onClose}>
              {managedMode ? "Cancel" : "Close"}
            </Button>
            {managedMode && (
              <Button
                variant="contained"
                color={isDelete ? "error" : "primary"}
                onClick={handleManagedSubmit}
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
            )}
          </Box>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

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
import { Copy, Eye, EyeOff, Plus } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useFormValidation,
} from "@agent-management-platform/views";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  useListDataPlanes,
  useCheckThunderUrlAvailability,
} from "@agent-management-platform/api-client";
import { globalConfig, type DataPlane } from "@agent-management-platform/types";
import {
  getAgentManagerUrl,
  getAmpVersionHelm,
  getIsolationTierMeta,
  getRawScriptUrl,
} from "@agent-management-platform/shared-component";
import {
  createEnvironmentSchema,
  type CreateEnvironmentFormValues,
  type IsolationTier,
} from "../form/environmentSchema";

const TOKEN_MASK = "•••••••••••••••";

// docsUrl is optional configuration. Leave the guide links undefined when it is
// unset rather than falling back to a relative path, which would resolve against
// the console's own origin instead of the documentation site.
const GVISOR_ISOLATION_DOCS_URL = globalConfig.docsUrl
  ? `${globalConfig.docsUrl}/guides/isolation-tiers/gvisor/`
  : undefined;
const KATA_ISOLATION_DOCS_URL = globalConfig.docsUrl
  ? `${globalConfig.docsUrl}/guides/isolation-tiers/kata/`
  : undefined;

// Per-tier copy for the picker and the pre-deploy node-requirement warning.
// runc has no warning: it is the default and needs no extra cluster setup.
const ISOLATION_TIER_OPTIONS: {
  value: IsolationTier;
  label: string;
  warning?: string;
  docsUrl?: string;
  docsLabel?: string;
}[] = [
  { value: "runc", label: "Sandbox Level 1 — runc (default)" },
  {
    value: "gvisor",
    label: "Sandbox Level 2 — gVisor",
    warning:
      "gVisor environments need a dedicated x86_64 node with the gVisor (runsc) runtime installed before agents can be deployed. Set up the node first — see the ",
    docsUrl: GVISOR_ISOLATION_DOCS_URL,
    docsLabel: "gVisor setup guide",
  },
  {
    value: "kata",
    label: "Sandbox Level 3 — Kata Containers",
    warning:
      "Kata environments need a dedicated node with KVM support (nested virtualization) and the Kata runtime installed before agents can be deployed. Set up the node first — see the ",
    docsUrl: KATA_ISOLATION_DOCS_URL,
    docsLabel: "Kata setup guide",
  },
];

interface CreateEnvironmentDrawerProps {
  open: boolean;
  onClose: () => void;
  orgId: string;
}

const DEFAULT_FORM: CreateEnvironmentFormValues = {
  name: "",
  displayName: "",
  description: "",
  dataplaneRef: "",
  dnsPrefix: "",
  isProduction: false,
  isolationTier: "runc",
  thunderHandle: "",
};

function deriveNameFromDisplayName(displayName: string): string {
  return displayName
    .toLowerCase()
    .replace(/\s+/g, "-")
    .replace(/[^a-z0-9-]/g, "")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

// Suggests "<name>-idp" as a starting point for the Thunder handle — truncated
// so the result never exceeds the backend's 63-char limit. Purely a default;
// the field stays fully editable and diverges from this suggestion the moment
// the user types into it directly (see the "in sync" checks below).
const THUNDER_HANDLE_SUFFIX = "-idp";
function deriveThunderHandleFromName(name: string): string {
  if (!name) return "";
  const maxNameLen = 63 - THUNDER_HANDLE_SUFFIX.length;
  return `${name.slice(0, maxNameLen)}${THUNDER_HANDLE_SUFFIX}`;
}

function buildScript(
  name: string,
  displayName: string,
  isProduction: boolean,
  isolationTier: IsolationTier,
  token: string,
  thunderHandle: string,
): string {
  // Cluster-internal addresses the gateway uses to reach Agent Manager. Sourced
  // from runtime config so the same drawer renders the right values for
  // docker-compose (host.docker.internal) vs in-cluster (svc.cluster.local)
  // deployments. The script appends /api/v1 and /auth/external/jwks.json to the
  // base URL itself, so we only need to pipe these two values through.
  const internalBase = globalConfig.agentManagerInternalBaseUrl?.trim();
  const internalCp = globalConfig.agentManagerInternalCpHost?.trim();
  // Base domain env-Thunder instances are hosted under, and whether this
  // deployment serves them over TLS. Without these, add-environment.sh falls
  // back to its own "amp.localhost"/non-TLS defaults regardless of the actual
  // deployment, producing a broken env-Thunder URL for any non-local-dev
  // install (e.g. a VM/production deployment with its own base domain).
  const thunderHostBaseDomain = globalConfig.idpHostBaseDomain?.trim();
  const tlsEnabled = globalConfig.tlsEnabled;
  // Required by add-environment.sh: the gateway chart version, pinned to the
  // platform release version so an added environment runs the same gateway chart.

  const chartVersion = getAmpVersionHelm();

  // The env-creation script chains into the Thunder provisioning script to also
  // stand up this environment's identity (Thunder) instance. Pass the version-matched
  // URL so the chained call fetches the script from the same git ref as this one.
  const lines = [
    `curl -fsSL ${getRawScriptUrl("add-environment.sh")} \\`,
    `  | ENV_NAME=${name || "<env-name>"} \\`,
    `    DISPLAY_NAME="${displayName || "<display-name>"}" \\`,
    ...(isolationTier !== "runc"
      ? [`    ISOLATION_TIER=${isolationTier} \\`]
      : []),
    `    AGENT_MANAGER_TOKEN=${token} \\`,
    `    AGENT_MANAGER_URL=${getAgentManagerUrl()} \\`,
    `    CHART_VERSION=${chartVersion || "<chart-version>"} \\`,
    ...(isProduction ? ["    IS_PRODUCTION=true \\"] : []),
    ...(internalBase
      ? [`    AGENT_MANAGER_INTERNAL_BASE_URL=${internalBase} \\`]
      : []),
    ...(internalCp ? [`    AGENT_MANAGER_INTERNAL_CP=${internalCp} \\`] : []),
    ...(thunderHostBaseDomain
      ? [`    THUNDER_HOST_BASE_DOMAIN=${thunderHostBaseDomain} \\`]
      : []),
    ...(tlsEnabled !== undefined ? [`    TLS_ENABLED=${tlsEnabled} \\`] : []),
    `    THUNDER_SCRIPT_URL=${getRawScriptUrl("add-environment-thunder.sh")} \\`,
    // THUNDER_HANDLE replaces the guessable <org>-<env> segment of this
    // environment's env-Thunder URL with an unguessable, user-chosen label —
    // omitted entirely (not an empty string) when unset, so
    // add-environment-thunder.sh's own "unset" default behavior applies.
    ...(thunderHandle ? [`    THUNDER_HANDLE=${thunderHandle} \\`] : []),
    "    bash",
  ];
  return lines.join("\n");
}

// amp.localhost is the local-dev default base domain (config.ThunderHostBaseDomain /
// THUNDER_HOST_BASE_DOMAIN) — the handle sits directly under it, with no fixed
// subdomain segment in between. Deployments (VM/production) publish their own
// base domain via globalConfig.idpHostBaseDomain; this is only the
// fallback for deployments that haven't set it.
const THUNDER_HOST_PREVIEW_DOMAIN =
  globalConfig.idpHostBaseDomain?.trim() || "amp.localhost";

export function CreateEnvironmentDrawer({
  open,
  onClose,
  orgId,
}: CreateEnvironmentDrawerProps) {
  const [formData, setFormData] =
    useState<CreateEnvironmentFormValues>(DEFAULT_FORM);
  const [showToken, setShowToken] = useState(false);
  const [resolvedToken, setResolvedToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const { errors, validateField, setFieldError } =
    useFormValidation<CreateEnvironmentFormValues>(createEnvironmentSchema);

  const { getToken } = useAuthHooks();
  const { data: dataPlanes } = useListDataPlanes({ orgName: orgId });
  const planes = dataPlanes ?? [];

  // Debounced so the availability check fires once typing pauses, not on
  // every keystroke — it only ever runs once the value already passes local
  // format validation (see the `enabled` gate below), so this never fires on
  // an obviously-invalid handle.
  const [debouncedThunderHandle, setDebouncedThunderHandle] = useState("");
  useEffect(() => {
    const timer = setTimeout(
      () => setDebouncedThunderHandle(formData.thunderHandle ?? ""),
      400,
    );
    return () => clearTimeout(timer);
  }, [formData.thunderHandle]);

  const thunderHandleFormatValid =
    !!formData.thunderHandle && !errors.thunderHandle;
  const { data: thunderHandleAvailability, isFetching: checkingThunderHandle } =
    useCheckThunderUrlAvailability(
      { orgName: orgId },
      { handle: debouncedThunderHandle },
      { enabled: thunderHandleFormatValid && debouncedThunderHandle === formData.thunderHandle },
    );
  const thunderHandleTaken =
    thunderHandleFormatValid &&
    debouncedThunderHandle === formData.thunderHandle &&
    thunderHandleAvailability?.available === false;

  useEffect(() => {
    if (open) {
      setFormData(DEFAULT_FORM);
      setShowToken(false);
      setResolvedToken(null);
      setCopied(false);
      setDebouncedThunderHandle("");
    }
  }, [open]);

  useEffect(() => {
    if (!formData.dataplaneRef && planes.length > 0) {
      setFormData((prev) => ({ ...prev, dataplaneRef: planes[0].name }));
    }
  }, [planes, formData.dataplaneRef]);

  const handleChange = useCallback(
    (field: keyof CreateEnvironmentFormValues, value: string | boolean) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value } as CreateEnvironmentFormValues;
        setFieldError(field, validateField(field, next[field], next));
        return next;
      });
    },
    [validateField, setFieldError],
  );

  const handleDisplayNameChange = useCallback(
    (value: string) => {
      setFormData((prev) => {
        const derivedName = deriveNameFromDisplayName(value);
        const nameInSync =
          prev.name === "" ||
          prev.name === deriveNameFromDisplayName(prev.displayName);
        const newName = nameInSync ? derivedName : prev.name;
        const handleInSync =
          prev.thunderHandle === "" ||
          prev.thunderHandle === deriveThunderHandleFromName(prev.name);
        const newThunderHandle = handleInSync
          ? deriveThunderHandleFromName(newName)
          : prev.thunderHandle;
        const next = {
          ...prev,
          displayName: value,
          name: newName,
          dnsPrefix: newName,
          thunderHandle: newThunderHandle,
        };
        setFieldError("displayName", validateField("displayName", value, next));
        setFieldError("name", validateField("name", newName, next));
        setFieldError(
          "thunderHandle",
          validateField("thunderHandle", newThunderHandle, next),
        );
        return next;
      });
    },
    [validateField, setFieldError],
  );

  const handleNameChange = useCallback(
    (value: string) => {
      setFormData((prev) => {
        const handleInSync =
          prev.thunderHandle === "" ||
          prev.thunderHandle === deriveThunderHandleFromName(prev.name);
        const newThunderHandle = handleInSync
          ? deriveThunderHandleFromName(value)
          : prev.thunderHandle;
        const next = {
          ...prev,
          name: value,
          dnsPrefix: value,
          thunderHandle: newThunderHandle,
        };
        setFieldError("name", validateField("name", value, next));
        setFieldError(
          "thunderHandle",
          validateField("thunderHandle", newThunderHandle, next),
        );
        return next;
      });
    },
    [validateField, setFieldError],
  );

  // Editable: once the user types into this field directly, it diverges from
  // the name-derived suggestion above and is never overwritten again.
  // Lowercased as typed, matching the format the backend requires.
  const handleThunderHandleChange = useCallback(
    (value: string) => {
      const lowered = value.toLowerCase();
      setFormData((prev) => {
        const next = { ...prev, thunderHandle: lowered };
        setFieldError("thunderHandle", validateField("thunderHandle", lowered, next));
        return next;
      });
    },
    [validateField, setFieldError],
  );

  const handleToggleToken = useCallback(async () => {
    if (showToken) {
      setShowToken(false);
      setResolvedToken(null);
    } else {
      try {
        const token = await getToken();
        setResolvedToken(token);
        setShowToken(true);
      } catch {
        // silently fail
      }
    }
  }, [showToken, getToken]);

  const handleCopy = useCallback(async () => {
    try {
      const token = resolvedToken ?? (await getToken());
      const script = buildScript(
        formData.name,
        formData.displayName,
        formData.isProduction ?? false,
        formData.isolationTier ?? "runc",
        token,
        formData.thunderHandle ?? "",
      );
      await navigator.clipboard.writeText(script);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // silently fail
    }
  }, [
    resolvedToken,
    getToken,
    formData.name,
    formData.displayName,
    formData.isProduction,
    formData.isolationTier,
    formData.thunderHandle,
  ]);

  const displayScript = useMemo(
    () =>
      buildScript(
        formData.name,
        formData.displayName,
        formData.isProduction ?? false,
        formData.isolationTier ?? "runc",
        showToken && resolvedToken ? resolvedToken : TOKEN_MASK,
        formData.thunderHandle ?? "",
      ),
    [
      formData.name,
      formData.displayName,
      formData.isProduction,
      formData.isolationTier,
      formData.thunderHandle,
      showToken,
      resolvedToken,
    ],
  );

  const selectedTier = ISOLATION_TIER_OPTIONS.find(
    (t) => t.value === (formData.isolationTier ?? "runc"),
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Plus size={24} />}
        title="Create Environment"
        onClose={onClose}
      />
      <DrawerContent>
        <Stack spacing={3}>
          <Typography variant="body2" color="text.secondary">
            Environments are provisioned by a script that creates the
            environment in Agent Manager, installs its API Platform Gateway, and
            stands up its dedicated identity (Thunder) instance via Helm. Fill
            in the details below, then copy and run the command in a terminal
            with <code>kubectl</code> and <code>helm</code> configured against
            your cluster.
          </Typography>

          <Stack spacing={2}>
            {planes.length > 1 && (
              <FormControl fullWidth error={Boolean(errors.dataplaneRef)}>
                <FormLabel required>Data Plane</FormLabel>
                <Select
                  size="small"
                  value={formData.dataplaneRef}
                  onChange={(e) =>
                    handleChange("dataplaneRef", e.target.value as string)
                  }
                  error={Boolean(errors.dataplaneRef)}
                >
                  {planes.map((p: DataPlane) => (
                    <MenuItem key={p.name} value={p.name}>
                      {p.displayName || p.name}
                    </MenuItem>
                  ))}
                </Select>
                {errors.dataplaneRef && (
                  <Typography variant="caption" color="error">
                    {errors.dataplaneRef}
                  </Typography>
                )}
              </FormControl>
            )}

            <FormControl fullWidth error={Boolean(errors.displayName)}>
              <FormLabel required>Display Name</FormLabel>
              <TextField
                size="small"
                fullWidth
                value={formData.displayName}
                onChange={(e) => handleDisplayNameChange(e.target.value)}
                placeholder="e.g., Production"
                error={Boolean(errors.displayName)}
                helperText={errors.displayName}
              />
            </FormControl>

            <FormControl fullWidth error={Boolean(errors.name)}>
              <FormLabel>Name</FormLabel>
              <TextField
                size="small"
                fullWidth
                value={formData.name}
                onChange={(e) => handleNameChange(e.target.value)}
                placeholder="e.g., production"
                error={Boolean(errors.name)}
                helperText={
                  errors.name ??
                  "The resource name used by the API. Generated automatically and guaranteed unique."
                }
              />
            </FormControl>

            <FormControl fullWidth>
              <FormLabel>Isolation Tier</FormLabel>
              <Select
                size="small"
                value={formData.isolationTier ?? "runc"}
                onChange={(e) =>
                  handleChange("isolationTier", e.target.value as string)
                }
              >
                {ISOLATION_TIER_OPTIONS.map((tier) => {
                  const TierIcon = getIsolationTierMeta(tier.value).icon;
                  return (
                    <MenuItem key={tier.value} value={tier.value}>
                      <Stack direction="row" spacing={1} alignItems="center">
                        <TierIcon size={14} />
                        <span>{tier.label}</span>
                      </Stack>
                    </MenuItem>
                  );
                })}
              </Select>
              <Typography variant="caption" color="text.secondary">
                Container runtime isolation for agents deployed to this
                environment.
              </Typography>
            </FormControl>

            <FormControl
              fullWidth
              error={Boolean(errors.thunderHandle) || thunderHandleTaken}
            >
              <FormLabel>Identity Service Handle</FormLabel>
              <TextField
                size="small"
                fullWidth
                value={formData.thunderHandle ?? ""}
                onChange={(e) => handleThunderHandleChange(e.target.value)}
                placeholder="Leave blank to auto-generate"
                error={Boolean(errors.thunderHandle) || thunderHandleTaken}
              />
              <Typography
                variant="caption"
                color={
                  errors.thunderHandle || thunderHandleTaken
                    ? "error"
                    : "text.secondary"
                }
                sx={{ mt: 0.5 }}
              >
                {errors.thunderHandle ??
                  (thunderHandleTaken
                    ? "This handle is already in use — choose a different one."
                    : checkingThunderHandle && thunderHandleFormatValid
                      ? "Checking availability…"
                      : "Used as the handle in this environment's Thunder identity URL - leave blank to auto-generate.")}
              </Typography>
              <Typography
                variant="caption"
                color="text.secondary"
                sx={{ mt: 0.5, fontFamily: "monospace" }}
              >
                {/* Matches thunderExternalOrigin (naming.go): TLS deployments serve
                    on the standard port, local/non-TLS dev serves on 8080. */}
                {`Preview: ${globalConfig.tlsEnabled ? "https" : "http"}://${
                  formData.thunderHandle || "auto-generated"
                }.${THUNDER_HOST_PREVIEW_DOMAIN}${globalConfig.tlsEnabled ? "" : ":8080"}`}
              </Typography>
            </FormControl>

            <FormControlLabel
              control={
                <Checkbox
                  checked={formData.isProduction ?? false}
                  onChange={(e) =>
                    handleChange("isProduction", e.target.checked)
                  }
                />
              }
              label="Production environment"
            />
          </Stack>

          {selectedTier?.warning && (
            <Alert severity="warning">
              {selectedTier.warning}
              {selectedTier.docsUrl ? (
                <Typography
                  component="a"
                  href={selectedTier.docsUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  sx={{ color: "primary.main" }}
                >
                  {selectedTier.docsLabel}
                </Typography>
              ) : (
                selectedTier.docsLabel
              )}
              .
            </Alert>
          )}

          <Stack spacing={1}>
            <Typography variant="body2">
              Run from the root of your repo clone:
            </Typography>
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
              <Box
                sx={{
                  position: "absolute",
                  top: 6,
                  right: 6,
                  display: "flex",
                  gap: 0.5,
                }}
              >
                <Tooltip title={showToken ? "Hide token" : "Show token"}>
                  <IconButton
                    size="small"
                    onClick={handleToggleToken}
                    sx={{ color: "grey.400" }}
                  >
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

          <Alert severity="info">
            Once the script completes, the new environment will appear in the
            list. The script is idempotent — safe to re-run.
          </Alert>

          <Box display="flex" justifyContent="flex-end">
            <Button variant="outlined" color="inherit" onClick={onClose}>
              Close
            </Button>
          </Box>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

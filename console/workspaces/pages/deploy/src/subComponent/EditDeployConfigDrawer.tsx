/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
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

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Button,
  CircularProgress,
  Divider,
  Form,
  MenuItem,
  Select,
  Stack,
  Switch,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, RefreshCw, SlidersVertical } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  EnvFileUploadButton,
  EnvVariableEditor,
  FileMountEditor,
  TokenExpirySelector,
  DEFAULT_TOKEN_EXPIRY,
  useSnackBar,
} from "@agent-management-platform/views";
import {
  ALLOWED,
  RestrictedAction,
  useAgentEnvironmentAccess,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import {
  extractServerErrorMessage,
  MAX_SNACKBAR_REASON_LENGTH,
  useAgentBuildOptions,
  useDeployAgent,
  useGetAgentConfigurations,
  useRegenerateTracingToken,
  useUpdateAgentConfigurations,
  useUpdateAgentDeploySettings,
} from "@agent-management-platform/api-client";
import type {
  EnvironmentVariable,
  UpdateAgentDeploySettingsRequest,
} from "@agent-management-platform/types";
import { compatibleInstrumentationVersions, pickInstrumentationVersion } from "../utils/instrumentation";
import { isStoredSecret, sortSystemLast, toSubmittableEnv } from "../utils/envVars";
import {
  type FileMountRow,
  newFileMountRow,
  seedFileMountRows,
  toFileMount,
} from "../utils/fileMounts";
import { SecurityConfigSections, type SecurityConfigHandle } from "./SecurityConfigSections";

export interface EditDeployConfigDrawerProps {
  open: boolean;
  onClose: () => void;
  orgName: string;
  projName: string;
  agentName: string;
  environment: string;
  title: string;
  /**
   * "update" — call PUT /configurations to swap env/files on an already-deployed
   * release binding (no imageId needed). Used by DeployCard's Configure button.
   * "deploy" — call POST /deployments with imageId, env, files. Used by BuildCard's
   * "Configure & Deploy" button, which is creating the first deployment.
   */
  mode: "update" | "deploy";
  /** Required when mode === "deploy". */
  imageId?: string;
  // Tracing/instrumentation context — supplied by DeployCard for deployed agents so the
  // "Tracing - Instrumentation" section can render, and whether the Python-only
  // instrumentation version selector is offered within it. Both omitted for Docker
  // agents, which have no instrumentation trait and so get no section at all.
  isPythonBuildpack?: boolean;
  isBallerinaBuildpack?: boolean;
  agentPythonVersion?: string;
  // When true (API agents, update mode), the CORS + Endpoint Authentication sections render at the
  // top of the drawer. Omitted by BuildCard (initial deploy) and non-API agents.
  isApiAgent?: boolean;
}

export function EditDeployConfigDrawer({
  open,
  onClose,
  imageId,
  orgName,
  projName,
  agentName,
  environment,
  title,
  mode,
  isPythonBuildpack,
  isBallerinaBuildpack,
  agentPythonVersion,
  isApiAgent,
}: EditDeployConfigDrawerProps) {
  const { pushSnackBar } = useSnackBar();
  const { addConfirmation } = useConfirmationDialog();

  // CORS + Endpoint Authentication live in a child component that owns its own state; we collect
  // its payload on Apply via the imperative handle.
  const showSecurity = mode === "update" && !!isApiAgent;

  // In deploy mode this drawer's Apply calls the deploy route, which is gated on
  // the target environment's tier. The buttons that open it are gated too, but
  // the drawer also opens straight from a ?deployPanel=open link, so the control
  // that actually deploys carries the check as well.
  const environmentAccess = useAgentEnvironmentAccess(orgName);
  const deployAccess =
    mode === "deploy" ? environmentAccess(environment) : ALLOWED;
  const securityRef = useRef<SecurityConfigHandle>(null);
  const [securityValid, setSecurityValid] = useState(true);

  const { data: configurations } = useGetAgentConfigurations(
    { orgName, projName, agentName },
    { environment },
  );

  const [env, setEnv] = useState<EnvironmentVariable[]>([]);
  const [files, setFiles] = useState<FileMountRow[]>([]);

  // Tracing section: the environment card's drawer only. Offered for every language the
  // backend can instrument — Python and Ballerina alike, which is what makes it reachable
  // for kind-based agents. The version selector below stays Python-only: the
  // instrumentation image tags encode the Python minor (e.g. 0.4.1-python3.11).
  const showTracing =
    mode === "update" && (!!isPythonBuildpack || !!isBallerinaBuildpack);
  const [tracingEnabled, setTracingEnabled] = useState(false);
  const [instrumentationVersion, setInstrumentationVersion] = useState<string>("");
  const [versionDirty, setVersionDirty] = useState(false);
  const [tokenExpiry, setTokenExpiry] = useState<string>(DEFAULT_TOKEN_EXPIRY);

  const { data: buildOptions } = useAgentBuildOptions({ orgName });
  const compatibleInstrumentation = useMemo(
    () => compatibleInstrumentationVersions(buildOptions, agentPythonVersion),
    [buildOptions, agentPythonVersion],
  );
  const versionInCompatibleSet = compatibleInstrumentation.some(
    (v) => v.version === instrumentationVersion,
  );

  // Seed edit state once per open cycle. Guarding with a ref prevents a background refetch of
  // configurations (window-focus / stale-time) from wiping unsaved edits while the drawer is open.
  const seededRef = useRef(false);
  useEffect(() => {
    if (!open) {
      seededRef.current = false;
      return;
    }
    if (seededRef.current || !configurations) return;
    const cfg = configurations.configurations;
    setEnv(sortSystemLast(cfg?.env?.map(
      (e) => ({
        key: e.key,
        value: e.value ?? "",
        isSensitive: e.isSensitive,
        secretRef: e.secretRef,
        isSystem: e.isSystem,
      }),
    ) ?? []));
    setFiles(seedFileMountRows(cfg?.files));
    setTracingEnabled(configurations.enableAutoInstrumentation ?? false);
    setInstrumentationVersion("");
    setVersionDirty(false);
    setTokenExpiry(DEFAULT_TOKEN_EXPIRY);
    seededRef.current = true;
  }, [open, configurations]);

  // Seed the version selector for display once the catalog has loaded; re-seed while the current
  // value is not compatible (self-corrects a stale seed without clobbering a valid user choice).
  useEffect(() => {
    if (!open || !isPythonBuildpack || !buildOptions) return;
    if (versionInCompatibleSet) return;
    setInstrumentationVersion(
      pickInstrumentationVersion(
        compatibleInstrumentation,
        configurations?.instrumentationVersion,
        buildOptions.instrumentation.defaultVersion,
      ),
    );
  }, [
    open, isPythonBuildpack, buildOptions, compatibleInstrumentation, configurations,
    versionInCompatibleSet,
  ]);

  const { mutate: deployAgent, isPending: isDeploying } = useDeployAgent();
  const { mutate: updateConfigs, isPending: isUpdating } = useUpdateAgentConfigurations();
  const { mutate: updateDeploySettings, isPending: isUpdatingSettings } =
    useUpdateAgentDeploySettings();
  const { mutate: regenerateToken, isPending: isRegenerating } = useRegenerateTracingToken();
  const isPending = isDeploying || isUpdating || isUpdatingSettings;

  const handleSave = useCallback(() => {
    const validEnv = toSubmittableEnv(env);
    const validFiles = files
      .filter((f) => f.key && f.mountPath)
      .map(toFileMount);

    if (mode === "update") {
      if (showSecurity && securityRef.current && !securityRef.current.validate()) {
        pushSnackBar({ message: "Fix the highlighted security settings before applying", type: "error" });
        return;
      }

      const applyConfigs = () =>
        updateConfigs(
          {
            params: { orgName, projName, agentName },
            body: { environmentName: environment, env: validEnv, files: validFiles },
          },
          { onSuccess: () => onClose() },
        );

      // Combine CORS/Auth (security) and tracing into a single deploy-settings call. The version is
      // sent only when the user changed it to a compatible value while tracing is on — otherwise
      // omitted so the backend preserves the existing pin (an unpinned agent keeps the default).
      const deploySettingsBody: UpdateAgentDeploySettingsRequest = {
        environmentName: environment,
        ...(showSecurity && securityRef.current ? securityRef.current.buildBody() : {}),
        ...(showTracing && {
          enableAutoInstrumentation: tracingEnabled,
          ...(isPythonBuildpack && tracingEnabled && versionDirty && versionInCompatibleSet
            && instrumentationVersion
            ? { instrumentationVersion }
            : {}),
        }),
      };

      if (showSecurity || showTracing) {
        updateDeploySettings(
          { params: { orgName, projName, agentName }, body: deploySettingsBody },
          { onSuccess: applyConfigs },
        );
      } else {
        applyConfigs();
      }
      return;
    }

    if (!imageId) {
      pushSnackBar({ message: "imageId is required for the initial deploy", type: "error" });
      return;
    }
    deployAgent(
      {
        params: { orgName, projName, agentName },
        body: {
          imageId,
          ...(validEnv.length && { env: validEnv }),
          ...(validFiles.length && { files: validFiles }),
        },
      },
      { onSuccess: () => onClose() },
    );
  }, [
    mode, env, files, environment, imageId, orgName, projName, agentName,
    showSecurity, showTracing, tracingEnabled, instrumentationVersion, versionDirty,
    versionInCompatibleSet, isPythonBuildpack,
    deployAgent, updateConfigs, updateDeploySettings, onClose, pushSnackBar,
  ]);

  // Regenerate mints + stores the new key immediately (no pre-confirm). The key only takes effect
  // once the workload restarts, so on success we prompt to Apply — confirming runs the same
  // Apply as the button (handleSave), which restarts the agent via the standard config path.
  const handleRegenerateToken = useCallback(() => {
    regenerateToken(
      {
        params: { orgName, projName, agentName },
        body: { environmentName: environment, expiresIn: tokenExpiry },
      },
      {
        onSuccess: (res) => {
          const expires = new Date(res.expiresAt * 1000).toLocaleDateString();
          addConfirmation({
            analytics: { entity: "deploy-config", action: "apply-restart" },
            title: "Apply to restart the agent?",
            description:
              `Tracing API key regenerated (expires ${expires}). Apply the configuration to ` +
              `restart the agent in ${environment} so the new key takes effect. Previously issued ` +
              "keys remain valid until they expire.",
            confirmButtonText: "Apply",
            confirmButtonColor: "warning",
            onConfirm: () => handleSave(),
          });
        },
        // useDeployAgent/useUpdateAgentConfigurations/useUpdateAgentDeploySettings already
        // show the server's message (+ reason, when the backend sends one) via their own
        // showError:true default — useRegenerateTracingToken suppresses its generic one
        // (showError:false) instead because its success path is custom (an expiry-aware
        // confirmation), so it needs its own error snackbar here.
        onError: (error: unknown) => {
          pushSnackBar({
            message:
              extractServerErrorMessage(error, { maxReasonLength: MAX_SNACKBAR_REASON_LENGTH }) ??
              "Failed to regenerate tracing token",
            type: "error",
          });
        },
      },
    );
  }, [
    regenerateToken, orgName, projName, agentName, environment, tokenExpiry,
    addConfirmation, handleSave, pushSnackBar,
  ]);

  // ── Env handlers ─────────────────────────────────────────────────────────
  const handleAddEnv = useCallback(() => {
    setEnv((prev) => [{ key: "", value: "", isSensitive: false }, ...prev]);
  }, []);

  const handleEnvChange = useCallback(
    (index: number, field: "key" | "value" | "isSensitive", value: string | boolean) => {
      // secretRef is intentionally preserved while editing so cancelling an edit
      // can restore the original masked secret. Submit decides whether to send
      // the new value or fall back to secretRef (see the save handler).
      setEnv((prev) =>
        prev.map((item, i) => (i === index ? { ...item, [field]: value } : item)),
      );
    },
    [],
  );

  const handleRemoveEnv = useCallback((index: number) => {
    setEnv((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const handleEnvFileParsed = useCallback((entries: { key: string; value: string }[]) => {
    setEnv((prev) => {
      const next = [...prev];
      for (const { key, value } of entries) {
        const existingIndex = next.findIndex((e) => e.key === key);
        if (existingIndex !== -1) {
          // Never let an uploaded .env file shadow a system-injected key.
          if (next[existingIndex].isSystem) continue;
          next[existingIndex] = { ...next[existingIndex], key, value, secretRef: undefined };
        } else {
          next.push({ key, value, isSensitive: false });
        }
      }
      return sortSystemLast(next);
    });
  }, []);

  // ── File handlers ─────────────────────────────────────────────────────────
  const handleAddFile = useCallback(() => {
    setFiles((prev) => [newFileMountRow(), ...prev]);
  }, []);

  const handleFileChange = useCallback(
    (index: number, field: "key" | "mountPath" | "value", value: string) => {
      setFiles((prev) => prev.map((f, i) => (i === index ? { ...f, [field]: value } : f)));
    },
    [],
  );

  const handleRemoveFile = useCallback((index: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== index));
  }, []);

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<SlidersVertical size={24} />} title={title} onClose={onClose} />
      <DrawerContent>
        <Form.Stack spacing={3}>
          {showSecurity && (
            <SecurityConfigSections
              ref={securityRef}
              orgName={orgName}
              projName={projName}
              agentName={agentName}
              environment={environment}
              open={open}
              disabled={isPending}
              configurations={configurations}
              onValidityChange={setSecurityValid}
            />
          )}

          {showTracing && (
            <Form.Section>
              <Form.Header>Tracing - Instrumentation</Form.Header>
              <Stack spacing={2}>
                <Stack direction="row" justifyContent="space-between" alignItems="center">
                  <Typography variant="body2">Auto-Instrumentation</Typography>
                  <Switch
                    size="small"
                    checked={tracingEnabled}
                    disabled={isPending}
                    onChange={(_, checked) => setTracingEnabled(checked)}
                  />
                </Stack>

                {tracingEnabled && isPythonBuildpack && (
                  <Stack direction="row" justifyContent="space-between" alignItems="center">
                    <Typography variant="body2">Instrumentation Version</Typography>
                    {compatibleInstrumentation.length === 0 && buildOptions ? (
                      <Typography variant="caption" color="text.secondary">
                        None available for Python {agentPythonVersion ?? "runtime"}
                      </Typography>
                    ) : (
                      <Select
                        size="small"
                        value={versionInCompatibleSet ? instrumentationVersion : ""}
                        disabled={isPending || !buildOptions}
                        onChange={(e) => {
                          setInstrumentationVersion(e.target.value as string);
                          setVersionDirty(true);
                        }}
                        sx={{ minWidth: 200 }}
                      >
                        {compatibleInstrumentation.map((v) => (
                          <MenuItem key={v.version} value={v.version}>
                            {v.traceloopSdk
                              ? `${v.version} (OpenLLMetry v${v.traceloopSdk})`
                              : v.version}
                          </MenuItem>
                        ))}
                      </Select>
                    )}
                  </Stack>
                )}

                <Stack direction="row" justifyContent="space-between" alignItems="center">
                  <Stack>
                    <Typography variant="body2">Tracing API Key</Typography>
                    <Typography variant="caption" color="text.secondary">
                      The API key the agent uses to authenticate trace ingestion
                    </Typography>
                  </Stack>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <TokenExpirySelector
                      value={tokenExpiry}
                      onChange={setTokenExpiry}
                      disabled={isRegenerating}
                    />
                    <Button
                      size="small"
                      variant="outlined"
                      color="warning"
                      startIcon={
                        isRegenerating ? <CircularProgress size={14} /> : <RefreshCw size={14} />
                      }
                      onClick={handleRegenerateToken}
                      disabled={isRegenerating || isPending}
                    >
                      Regenerate
                    </Button>
                  </Stack>
                </Stack>
              </Stack>
            </Form.Section>
          )}

          <Form.Section>
            <Stack direction="row" justifyContent="space-between" alignItems="center">
              <Form.Header>Environment Variables</Form.Header>
              <Stack direction="row" gap={1}>
                <EnvFileUploadButton
                  onParsed={handleEnvFileParsed}
                  disabled={isPending || !seededRef.current}
                />
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<Plus size={14} />}
                  onClick={handleAddEnv}
                  disabled={isPending || !seededRef.current}
                >
                  Add
                </Button>
              </Stack>
            </Stack>
            {env.length === 0 ? (
              <Typography variant="body2" color="text.secondary">
                No environment variables. Click Add to define them.
              </Typography>
            ) : (
              <Stack spacing={1}>
                {env.map((item, index) => (
                  <EnvVariableEditor
                    key={index}
                    index={index}
                    keyValue={item.key}
                    valueValue={item.value}
                    isSensitive={item.isSensitive ?? false}
                    isExistingSecret={isStoredSecret(item)}
                    isSystem={item.isSystem}
                    onKeyChange={(v) => handleEnvChange(index, "key", v)}
                    onValueChange={(v) => handleEnvChange(index, "value", v)}
                    onSensitiveChange={(v) => handleEnvChange(index, "isSensitive", v)}
                    onRemove={() => handleRemoveEnv(index)}
                  />
                ))}
              </Stack>
            )}
          </Form.Section>

          <Form.Section>
            <Stack direction="row" justifyContent="space-between" alignItems="center">
              <Form.Header>File Mounts</Form.Header>
              <Button
                size="small"
                variant="outlined"
                startIcon={<Plus size={14} />}
                onClick={handleAddFile}
                disabled={isPending || !seededRef.current}
              >
                Add
              </Button>
            </Stack>
            {files.length === 0 ? (
              <Typography variant="body2" color="text.secondary">
                No file mounts. Click Add to define them.
              </Typography>
            ) : (
              <Stack spacing={1} divider={<Divider />}>
                {files.map((file, index) => (
                  <FileMountEditor
                    key={file.id}
                    keyValue={file.key}
                    mountPathValue={file.mountPath}
                    contentValue={file.value}
                    onKeyChange={(v) => handleFileChange(index, "key", v)}
                    onMountPathChange={(v) => handleFileChange(index, "mountPath", v)}
                    onContentChange={(v) => handleFileChange(index, "value", v)}
                    onRemove={() => handleRemoveFile(index)}
                  />
                ))}
              </Stack>
            )}
          </Form.Section>

          <Box display="flex" justifyContent="flex-end" gap={1}>
            <Button variant="outlined" onClick={onClose} disabled={isPending}>
              Cancel
            </Button>
            <RestrictedAction decision={deployAccess} feature="edit-deploy-config">
              <Button
                variant="contained"
                color="primary"
                onClick={handleSave}
                disabled={
                  isPending || isRegenerating || (showSecurity && !securityValid)
                }
                startIcon={isPending ? <CircularProgress size={16} /> : undefined}
              >
                {isPending ? "Applying..." : mode === "deploy" ? "Apply & Deploy" : "Apply"}
              </Button>
            </RestrictedAction>
          </Box>
        </Form.Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

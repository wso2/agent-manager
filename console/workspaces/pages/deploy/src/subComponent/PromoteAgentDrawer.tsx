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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Collapse,
  Divider,
  Form,
  FormControl,
  FormControlLabel,
  MenuItem,
  Select,
  Stack,
  Switch,
  Typography,
} from "@wso2/oxygen-ui";
import { ArrowUpFromLine, Plus } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  EnvFileUploadButton,
  EnvVariableEditor,
  FileMountEditor,
} from "@agent-management-platform/views";
import {
  extractServerErrorMessage,
  useAgentBuildOptions,
  usePromoteAgent,
  useGetAgent,
  useGetAgentConfigurations,
  useGetDeploymentPipeline,
  useListEnvironments,
} from "@agent-management-platform/api-client";
import type {
  Environment,
  EnvironmentVariable,
} from "@agent-management-platform/types";
import {
  RestrictedAction,
  useAgentEnvironmentAccess,
} from "@agent-management-platform/shared-component";
import {
  compatibleInstrumentationVersions,
  normalizePythonMinor,
  pickInstrumentationVersion,
} from "../utils/instrumentation";
import { isStoredSecret, sortSystemLast, toSubmittableEnv } from "../utils/envVars";
import {
  type FileMountRow,
  newFileMountRow,
  seedFileMountRows,
  toFileMount,
} from "../utils/fileMounts";

interface PromoteAgentDrawerProps {
  open: boolean;
  onClose: () => void;
  sourceEnvironment: Environment;
  orgId: string;
  projectId: string;
  agentId: string;
}

interface PromoteFormState {
  targetEnvironment: string;
  useConfigFromSourceEnv: boolean;
  env: EnvironmentVariable[];
  files: FileMountRow[];
  instrumentationVersion: string;
  // True once the user explicitly picks a version. When false, the version is
  // omitted from the promote request so the backend inherits the source env's
  // pin rather than overwriting the target with a display-only seed.
  instrumentationVersionDirty: boolean;
}

const DEFAULT_STATE: PromoteFormState = {
  targetEnvironment: "",
  useConfigFromSourceEnv: false,
  env: [],
  files: [],
  instrumentationVersion: "",
  instrumentationVersionDirty: false,
};

export function PromoteAgentDrawer({
  open,
  onClose,
  sourceEnvironment,
  orgId,
  projectId,
  agentId,
}: PromoteAgentDrawerProps) {
  const [formState, setFormState] = useState<PromoteFormState>(DEFAULT_STATE);

  const { data: pipeline } = useGetDeploymentPipeline({
    orgName: orgId,
    projName: projectId,
  });
  const { data: environments } = useListEnvironments({ orgName: orgId });
  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  // Instrumentation version selection is only relevant for Python buildpack agents.
  const isPythonBuildpack =
    agent?.build?.type === "buildpack" &&
    "buildpack" in (agent.build ?? {}) &&
    (agent.build as { buildpack?: { language?: string } }).buildpack?.language ===
      "python";
  const agentPythonVersion = normalizePythonMinor(
    (agent?.build as { buildpack?: { languageVersion?: string } } | undefined)
      ?.buildpack?.languageVersion,
  );

  // Server instrumentation catalog, shared with the create wizard.
  const { data: buildOptions } = useAgentBuildOptions({ orgName: orgId });
  const compatibleInstrumentation = useMemo(
    () => compatibleInstrumentationVersions(buildOptions, agentPythonVersion),
    [buildOptions, agentPythonVersion],
  );

  const envDisplayName = useCallback(
    (name: string) =>
      environments?.find((e) => e.name === name)?.displayName ?? name,
    [environments],
  );

  // Promotion is gated on the tier of the environment being promoted INTO, so
  // every target is decided separately: a caller may hold staging and not
  // production even though both are downstream of here.
  const targetAccess = useAgentEnvironmentAccess(orgId);
  const selectedTargetAccess = targetAccess(formState.targetEnvironment);

  const {
    mutateAsync: promoteAgent,
    isPending,
    error,
    reset: resetMutation,
  } = usePromoteAgent();

  const targetEnvOptions = useMemo(() => {
    if (!pipeline) return [];
    const path = pipeline.promotionPaths.find(
      (p) => p.sourceEnvironmentRef === sourceEnvironment.name,
    );
    return path?.targetEnvironmentRefs ?? [];
  }, [pipeline, sourceEnvironment.name]);

  // Existing configuration of the selected destination environment. Keyed on the
  // target env, so selecting a different target refetches that env's config.
  const { data: targetConfigs, isSuccess: targetConfigLoaded } =
    useGetAgentConfigurations(
      { orgName: orgId, projName: projectId, agentName: agentId },
      { environment: formState.targetEnvironment },
    );

  // Tracks which target env we've already pre-filled the editor for, so we fill
  // once per target rather than on every background refetch.
  const [filledForTarget, setFilledForTarget] = useState<string | null>(null);

  // True once the target's config has loaded AND been seeded into formState.
  // Gates Add/Upload so a user can't add/merge entries into the editor while
  // it's still holding the previous target's (or blank) state, which the
  // seed effect below would otherwise clobber once it fires.
  const targetConfigReady =
    targetConfigLoaded && filledForTarget === formState.targetEnvironment;

  // Pick a default target environment when the drawer opens, and clear state on close.
  useEffect(() => {
    if (!open) {
      setFilledForTarget(null);
      setFormState(DEFAULT_STATE);
      resetMutation();
      return;
    }
    setFormState((prev) =>
      prev.targetEnvironment
        ? prev
        : { ...prev, targetEnvironment: targetEnvOptions[0]?.name ?? "" },
    );
  }, [open, targetEnvOptions, resetMutation]);

  // Pre-fill the editor with the destination environment's existing config so the
  // user edits from its previous values rather than starting blank. System-injected
  // entries (isSystem=true) are included for visibility but rendered disabled by
  // EnvVariableEditor and excluded from the submit payload in handleSubmit, since
  // they're platform-managed rather than user-managed. We wait for the target's
  // query to settle (targetConfigLoaded) before filling, so switching to a target
  // with no config clears the previous target's values to empty rather than leaving
  // them stale. We fill once per target (tracked by filledForTarget) so a background
  // refetch of the same target doesn't clobber in-progress edits.
  useEffect(() => {
    if (!open) return;
    const target = formState.targetEnvironment;
    if (!target || filledForTarget === target || !targetConfigLoaded) return;
    const cfg = targetConfigs?.configurations;
    const displayEnv = sortSystemLast((cfg?.env ?? []).map((e) => ({
      key: e.key,
      value: e.value ?? "",
      isSensitive: e.isSensitive,
      secretRef: e.secretRef,
      isSystem: e.isSystem,
    })));
    setFormState((prev) => ({
      ...prev,
      env: displayEnv,
      files: seedFileMountRows(cfg?.files),
    }));
    setFilledForTarget(target);
  }, [
    open,
    formState.targetEnvironment,
    targetConfigLoaded,
    targetConfigs,
    filledForTarget,
  ]);

  // Seed the version selector for DISPLAY once both the agent (for its Python
  // version) and the catalog have loaded. Re-seed whenever the current value is
  // not in the compatible set (self-corrects a stale seed / a target-env change);
  // a valid user selection is always in the set, so this never clobbers it.
  useEffect(() => {
    if (!open || !buildOptions || !agent) return;
    setFormState((prev) => {
      const inSet = compatibleInstrumentation.some(
        (v) => v.version === prev.instrumentationVersion,
      );
      if (inSet) return prev;
      const seed = pickInstrumentationVersion(
        compatibleInstrumentation,
        agent?.configurations?.instrumentationVersion,
        buildOptions.instrumentation.defaultVersion,
      );
      return { ...prev, instrumentationVersion: seed };
    });
  }, [
    open,
    buildOptions,
    agent,
    compatibleInstrumentation,
  ]);

  const handleToggleUseSourceConfig = useCallback((checked: boolean) => {
    setFormState((prev) => ({ ...prev, useConfigFromSourceEnv: checked }));
  }, []);

  // secretRef is intentionally preserved while editing so cancelling an edit can
  // restore the original masked secret. Submit decides whether to send the new
  // value or fall back to secretRef (see handleSubmit).
  const handleEnvChange = useCallback(
    (
      index: number,
      field: "key" | "value" | "isSensitive",
      value: string | boolean,
    ) => {
      setFormState((prev) => ({
        ...prev,
        env: prev.env.map((item, i) =>
          i === index ? { ...item, [field]: value } : item,
        ),
      }));
    },
    [],
  );

  const handleAddEnv = useCallback(() => {
    setFormState((prev) => ({
      ...prev,
      env: [{ key: "", value: "", isSensitive: false }, ...prev.env],
    }));
  }, []);

  const handleRemoveEnv = useCallback((index: number) => {
    setFormState((prev) => ({
      ...prev,
      env: prev.env.filter((_, i) => i !== index),
    }));
  }, []);

  const handleEnvFileParsed = useCallback((entries: { key: string; value: string }[]) => {
    setFormState((prev) => {
      const nextEnv = [...prev.env];
      for (const { key, value } of entries) {
        const existingIndex = nextEnv.findIndex((e) => e.key === key);
        if (existingIndex !== -1) {
          // Never let an uploaded .env file shadow a system-injected key.
          if (nextEnv[existingIndex].isSystem) continue;
          nextEnv[existingIndex] = { ...nextEnv[existingIndex], key, value, secretRef: undefined };
        } else {
          nextEnv.push({ key, value, isSensitive: false });
        }
      }
      return { ...prev, env: sortSystemLast(nextEnv) };
    });
  }, []);

  const handleAddFile = useCallback(() => {
    setFormState((prev) => ({
      ...prev,
      files: [newFileMountRow(), ...prev.files],
    }));
  }, []);

  const handleFileChange = useCallback(
    (index: number, field: "key" | "mountPath" | "value", value: string) => {
      setFormState((prev) => ({
        ...prev,
        files: prev.files.map((f, i) =>
          i === index ? { ...f, [field]: value } : f,
        ),
      }));
    },
    [],
  );

  const handleRemoveFile = useCallback((index: number) => {
    setFormState((prev) => ({
      ...prev,
      files: prev.files.filter((_, i) => i !== index),
    }));
  }, []);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!formState.targetEnvironment) return;

      try {
        await promoteAgent({
          params: { orgName: orgId, projName: projectId, agentName: agentId },
          body: {
            sourceEnvironment: sourceEnvironment.name,
            targetEnvironment: formState.targetEnvironment,
            useConfigFromSourceEnv: formState.useConfigFromSourceEnv,
            ...(formState.useConfigFromSourceEnv
              ? {}
              : {
                  env: toSubmittableEnv(formState.env),
                  files: formState.files
                    .filter((f) => f.key && f.mountPath)
                    .map(toFileMount),
                  // Only send the version when the user explicitly picked a
                  // compatible one; otherwise omit it so the backend inherits
                  // the source env's pin rather than overwriting the target with
                  // a display-only seed.
                  ...(isPythonBuildpack &&
                  formState.instrumentationVersionDirty &&
                  formState.instrumentationVersion &&
                  compatibleInstrumentation.some(
                    (v) => v.version === formState.instrumentationVersion,
                  )
                    ? { instrumentationVersion: formState.instrumentationVersion }
                    : {}),
                }),
          },
        });
        onClose();
      } catch {
        // handled by error
      }
    },
    [
      formState,
      isPythonBuildpack,
      compatibleInstrumentation,
      promoteAgent,
      orgId,
      projectId,
      agentId,
      sourceEnvironment.name,
      onClose,
    ],
  );

  const errorMessage = useMemo(
    () => (error ? (extractServerErrorMessage(error) ?? "Failed to promote agent") : null),
    [error],
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<ArrowUpFromLine size={24} />}
        title={`Promote from ${sourceEnvironment.displayName ?? sourceEnvironment.name} Environment`}
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {errorMessage && (
              <Alert severity="error">
                <Typography variant="body2">{errorMessage}</Typography>
              </Alert>
            )}

            {!selectedTargetAccess.allowed && (
              <Alert severity="warning">
                <Typography variant="body2">
                  {selectedTargetAccess.reason}
                </Typography>
              </Alert>
            )}

            {targetEnvOptions.length > 1 && (
              <>
                <Form.Section>
                  <Form.Header>Target Environment</Form.Header>
                  <Form.Stack spacing={2}>
                    <FormControl fullWidth required>
                      <Select
                        size="small"
                        value={formState.targetEnvironment}
                        onChange={(e) =>
                          setFormState((prev) => ({
                            ...prev,
                            targetEnvironment: e.target.value as string,
                          }))
                        }
                        displayEmpty
                        disabled={isPending}
                      >
                        <MenuItem value="" disabled>
                          <em>Select target environment</em>
                        </MenuItem>
                        {targetEnvOptions.map((t) => (
                          <MenuItem
                            key={t.name}
                            value={t.name}
                            disabled={!targetAccess(t.name).allowed}
                          >
                            {envDisplayName(t.name)}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                  </Form.Stack>
                </Form.Section>

                <Divider />
              </>
            )}

            <Form.Section>
              <Form.Header>Configuration</Form.Header>
              <Form.Stack spacing={2}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={formState.useConfigFromSourceEnv}
                      onChange={(e) =>
                        handleToggleUseSourceConfig(e.target.checked)
                      }
                      disabled={isPending}
                    />
                  }
                  label={
                    <Stack>
                      <Typography variant="body2">
                        Use config from source environment
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        Inherit environment variables and file mounts from{" "}
                        {sourceEnvironment.displayName ??
                          sourceEnvironment.name}
                      </Typography>
                    </Stack>
                  }
                />

                <Collapse
                  in={!formState.useConfigFromSourceEnv}
                  timeout="auto"
                  unmountOnExit
                >
                  <Stack spacing={2}>
                    <Card variant="outlined">
                      <CardContent>
                        <Stack spacing={1.5}>
                          <Stack
                            direction="row"
                            justifyContent="space-between"
                            alignItems="center"
                          >
                            <Typography variant="h6">
                              Environment Variables
                            </Typography>
                            <Stack direction="row" gap={1}>
                              <EnvFileUploadButton
                                onParsed={handleEnvFileParsed}
                                disabled={isPending || !targetConfigReady}
                              />
                              <Button
                                size="small"
                                variant="outlined"
                                startIcon={<Plus size={14} />}
                                onClick={handleAddEnv}
                                disabled={isPending || !targetConfigReady}
                              >
                                Add
                              </Button>
                            </Stack>
                          </Stack>
                          {formState.env.length === 0 ? (
                            <Typography variant="body2" color="text.secondary">
                              No environment variables. Click Add to define
                              them.
                            </Typography>
                          ) : (
                            <Stack spacing={1}>
                              {formState.env.map((item, index) => (
                                <EnvVariableEditor
                                  key={index}
                                  index={index}
                                  keyValue={item.key}
                                  valueValue={item.value}
                                  isSensitive={item.isSensitive ?? false}
                                  isExistingSecret={isStoredSecret(item)}
                                  isSystem={item.isSystem}
                                  onKeyChange={(v) =>
                                    handleEnvChange(index, "key", v)
                                  }
                                  onValueChange={(v) =>
                                    handleEnvChange(index, "value", v)
                                  }
                                  onSensitiveChange={(v) =>
                                    handleEnvChange(index, "isSensitive", v)
                                  }
                                  onRemove={() => handleRemoveEnv(index)}
                                />
                              ))}
                            </Stack>
                          )}
                        </Stack>
                      </CardContent>
                    </Card>

                    <Card variant="outlined">
                      <CardContent>
                        <Stack spacing={1.5}>
                          <Stack
                            direction="row"
                            justifyContent="space-between"
                            alignItems="center"
                          >
                            <Typography variant="h6">File Mounts</Typography>
                            <Button
                              size="small"
                              variant="outlined"
                              startIcon={<Plus size={14} />}
                              onClick={handleAddFile}
                              disabled={isPending || !targetConfigReady}
                            >
                              Add
                            </Button>
                          </Stack>
                          {formState.files.length === 0 ? (
                            <Typography variant="body2" color="text.secondary">
                              No file mounts. Click Add to define them.
                            </Typography>
                          ) : (
                            <Stack spacing={1} divider={<Divider />}>
                              {formState.files.map((file, index) => (
                                <FileMountEditor
                                  key={file.id}
                                  keyValue={file.key}
                                  mountPathValue={file.mountPath}
                                  contentValue={file.value}
                                  onKeyChange={(v) =>
                                    handleFileChange(index, "key", v)
                                  }
                                  onMountPathChange={(v) =>
                                    handleFileChange(index, "mountPath", v)
                                  }
                                  onContentChange={(v) =>
                                    handleFileChange(index, "value", v)
                                  }
                                  onRemove={() => handleRemoveFile(index)}
                                />
                              ))}
                            </Stack>
                          )}
                        </Stack>
                      </CardContent>
                    </Card>

                    {isPythonBuildpack && (
                      <Card variant="outlined">
                        <CardContent>
                          <Stack spacing={1.5}>
                            <Typography variant="h6">
                              AMP Instrumentation Version
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                              Pins the init-container image and the bundled
                              OpenLLMetry SDK version for{" "}
                              {envDisplayName(formState.targetEnvironment)}.
                            </Typography>
                            {compatibleInstrumentation.length === 0 &&
                            buildOptions ? (
                              <Alert severity="info">
                                <Typography variant="body2">
                                  No AMP-provided instrumentation is available for
                                  Python {agentPythonVersion ?? "the selected version"}.
                                  You can still promote and instrument manually.
                                </Typography>
                              </Alert>
                            ) : (
                              <FormControl sx={{ minWidth: 240 }}>
                                <Select
                                  size="small"
                                  value={
                                    compatibleInstrumentation.some(
                                      (v) =>
                                        v.version ===
                                        formState.instrumentationVersion,
                                    )
                                      ? formState.instrumentationVersion
                                      : ""
                                  }
                                  disabled={isPending || !buildOptions}
                                  onChange={(e) =>
                                    setFormState((prev) => ({
                                      ...prev,
                                      instrumentationVersion: e.target
                                        .value as string,
                                      instrumentationVersionDirty: true,
                                    }))
                                  }
                                >
                                  {compatibleInstrumentation.map((v) => (
                                    <MenuItem key={v.version} value={v.version}>
                                      {v.traceloopSdk
                                        ? `${v.version} (OpenLLMetry v${v.traceloopSdk})`
                                        : v.version}
                                    </MenuItem>
                                  ))}
                                </Select>
                              </FormControl>
                            )}
                          </Stack>
                        </CardContent>
                      </Card>
                    )}
                  </Stack>
                </Collapse>
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
                color="inherit"
                onClick={onClose}
                disabled={isPending}
              >
                Cancel
              </Button>
              <RestrictedAction decision={selectedTargetAccess}>
                <Button
                  type="submit"
                  variant="contained"
                  color="primary"
                  disabled={isPending || !formState.targetEnvironment}
                >
                  {isPending ? "Promoting..." : "Promote"}
                </Button>
              </RestrictedAction>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}

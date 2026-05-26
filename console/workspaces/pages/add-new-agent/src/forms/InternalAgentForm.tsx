/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Alert, Checkbox, Collapse, Form, FormControl, FormControlLabel, FormHelperText, MenuItem, Select, Stack, TextField, Typography, CircularProgress } from "@wso2/oxygen-ui";
import { useEffect, useMemo, useCallback } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import {
  useAgentBuildOptions,
  useGenerateResourceName,
} from "@agent-management-platform/api-client";
import { globalConfig } from "@agent-management-platform/types";
import { InputInterface } from "../components/InputInterface";
import { EnvironmentVariable } from "../components/EnvironmentVariable";
import { FileMount } from "../components/FileMount";
import { GitSecretSelector } from "../components/GitSecretSelector";
import { LLMProviderSection } from "../components/LLMProviderSection";
import type { CreateAgentFormValues, LLMProviderFormEntry } from "../form/schema";
import { BuildpackIcon, useExternalConfigModules } from "@agent-management-platform/views";

interface InternalAgentFormProps {
  formData: CreateAgentFormValues;
  setFormData: React.Dispatch<React.SetStateAction<CreateAgentFormValues>>;
  errors: Record<string, string | undefined>;
  setFieldError: (
    field: keyof CreateAgentFormValues,
    error: string | undefined
  ) => void;
  validateField: (
    field: keyof CreateAgentFormValues,
    value: unknown,
    fullData?: CreateAgentFormValues
  ) => string | undefined;
  llmProviders: LLMProviderFormEntry[];
  setLLMProviders: React.Dispatch<React.SetStateAction<LLMProviderFormEntry[]>>;
}
const languageOptions = [
  { label: "Python", value: "python" },
  { label: "Docker", value: "docker" },
];

const MANUAL_INSTRUMENTATION_DOCS_URL =
  `${globalConfig.docsUrl ?? ""}${globalConfig.instrumentationDocLinks?.manualInstrumentation ?? ""}`;
const VERSION_MAPPING_DOCS_URL =
  `${globalConfig.docsUrl ?? ""}${globalConfig.instrumentationDocLinks?.versionMapping ?? ""}`;

export const InternalAgentForm = ({
  formData,
  setFormData,
  errors,
  setFieldError,
  validateField,
  llmProviders,
  setLLMProviders,
}: InternalAgentFormProps) => {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();
  const privateRepoConfigs = useExternalConfigModules("private-repo-support");
  const isPrivateRepoEnabled =
    privateRepoConfigs.length === 0 ||
    (privateRepoConfigs[0]?.value as { enabled?: boolean })
      ?.enabled !== false;

  const { mutate: generateName, isPending: isGeneratingName } = useGenerateResourceName({
    orgName: orgId,
  });

  // Fetch the platform's supported Python versions + the instrumentation
  // catalog. Until this resolves, the dropdowns render empty.
  const buildOptionsQuery = useAgentBuildOptions({ orgName: orgId ?? "" });
  const buildOptions = buildOptionsQuery.data;

  // Instrumentation entries compatible with the currently-selected Python.
  // Empty list when no AMP-provided instrumentation covers that Python —
  // the form then offers the manual-instrumentation fallback.
  const compatibleInstrumentation = useMemo(() => {
    if (!buildOptions) return [] as { version: string; pythonVersions: string[] }[];
    const py = formData.languageVersion;
    if (!py) return buildOptions.instrumentation.versions;
    return buildOptions.instrumentation.versions.filter((v) =>
      v.pythonVersions.includes(py),
    );
  }, [buildOptions, formData.languageVersion]);

  // Seed defaults when build options arrive, and normalise any value
  // that's no longer in the refreshed set (the catalog can change
  // mid-session if React Query refetches after a helm upgrade).
  useEffect(() => {
    if (!buildOptions) return;
    const supportedPython = new Set(buildOptions.python.supportedVersions);
    setFormData((prev) => {
      const next = { ...prev };
      let touched = false;
      if (prev.languageVersion == null || !supportedPython.has(prev.languageVersion)) {
        next.languageVersion = buildOptions.python.defaultVersion;
        touched = true;
      }
      if (prev.instrumentationVersion == null) {
        next.instrumentationVersion = buildOptions.instrumentation.defaultVersion;
        touched = true;
      }
      return touched ? next : prev;
    });
    // The second useEffect below handles instrumentationVersion staleness
    // via the python-compat filter, so we don't duplicate that check here.
  }, [buildOptions, setFormData]);

  // When the user changes Python, if the current instrumentation is no
  // longer compatible, reset to the platform default (if compatible),
  // else the newest compatible, else null (manual-instrumentation path).
  useEffect(() => {
    if (!buildOptions) return;
    const py = formData.languageVersion;
    if (!py) return;
    const compat = buildOptions.instrumentation.versions.filter((v) =>
      v.pythonVersions.includes(py),
    );
    const current = formData.instrumentationVersion;
    const isCurrentCompat =
      current != null && compat.some((c) => c.version === current);
    if (isCurrentCompat) return;
    let nextVersion: string | null = null;
    if (compat.length > 0) {
      const def = buildOptions.instrumentation.defaultVersion;
      nextVersion = compat.some((c) => c.version === def) ? def : compat[0].version;
    }
    setFormData((prev) => ({ ...prev, instrumentationVersion: nextVersion }));
  }, [
    buildOptions,
    formData.languageVersion,
    formData.instrumentationVersion,
    setFormData,
  ]);

  const handleFieldChange = useCallback(
    (field: keyof CreateAgentFormValues, value: unknown) => {
      setFormData(prevData => {
        const newData = { ...prevData, [field]: value };
        const error = validateField(field, value, newData);
        setFieldError(field, error);

        // When language changes, clear errors for conditional fields
        if (field === 'language') {
          if (value === 'python') {
            // Switching to Python - clear Docker errors
            setFieldError('dockerfilePath', undefined);
            // Re-validate Python fields
            const runCommandError = validateField('runCommand', newData.runCommand, newData);
            const languageVersionError = validateField('languageVersion', newData.languageVersion, newData);
            setFieldError('runCommand', runCommandError);
            setFieldError('languageVersion', languageVersionError);
          } else if (value === 'docker') {
            // Switching to Docker - clear Python errors
            setFieldError('runCommand', undefined);
            setFieldError('languageVersion', undefined);
            // Re-validate Docker fields
            const dockerfilePathError = validateField('dockerfilePath', newData.dockerfilePath, newData);
            setFieldError('dockerfilePath', dockerfilePathError);
          }
        }

        return newData;
      });
    },
    [setFormData, validateField, setFieldError]
  );

  // Create debounced function for name generation
  const debouncedGenerateName = useMemo(
    () =>
      debounce((name: string) => {
        if (name.length < 3) {
          handleFieldChange("name", "");
          return;
        }
        generateName({
          displayName: name,
          resourceType: 'agent',
          projectName: projectId,
        }, {
          onSuccess: (data: { name: string }) => {
            handleFieldChange("name", data.name);
          },
          onError: (error: unknown) => {
            // eslint-disable-next-line no-console
            console.error('Failed to generate name:', error);
          }
        });
      }, 500),
    [generateName, handleFieldChange, projectId]
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      debouncedGenerateName.cancel();
    };
  }, [debouncedGenerateName]);

  // Auto-generate name from display name using API with debounce
  useEffect(() => {
    if (formData.displayName && formData.displayName.length >= 3) {
      debouncedGenerateName(formData.displayName);
    } else {
      debouncedGenerateName.cancel();
      handleFieldChange("name", "");
    }
  }, [formData.displayName, handleFieldChange, debouncedGenerateName]);

  return (
    <Form.Stack spacing={3}>
      <Form.Section>
        <Form.Subheader>Agent Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.ElementWrapper label="Name" name="displayName">
            <TextField
              id="displayName"
              placeholder="e.g., Customer Support Agent"
              value={formData.displayName}
              onChange={(e) => handleFieldChange('displayName', e.target.value)}
              error={!!errors.displayName}
              helperText={
                isGeneratingName ? (
                  <Stack direction="row" alignItems="center" gap={1}>
                    <CircularProgress size={12} />
                    <Typography variant="caption">Validating name...</Typography>
                  </Stack>
                ) : (
                  errors.displayName || "A name for your agent"
                )
              }
              fullWidth
            />
          </Form.ElementWrapper>
          <Form.ElementWrapper label="Description (optional)" name="description">
            <TextField
              id="description"
              placeholder="Short description of what this agent does"
              multiline
              minRows={2}
              maxRows={6}
              value={formData.description || ''}
              onChange={(e) => handleFieldChange('description', e.target.value)}
              error={!!errors.description}
              helperText={errors.description}
              fullWidth
            />
          </Form.ElementWrapper>
        </Form.Stack>
      </Form.Section>

      <Form.Section>
        <Form.Subheader>Repository Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.ElementWrapper label="GitHub Repository" name="repositoryUrl">
            <TextField
              id="repositoryUrl"
              placeholder="https://github.com/username/repo"
              value={formData.repositoryUrl}
              onChange={(e) => handleFieldChange('repositoryUrl', e.target.value)}
              error={!!errors.repositoryUrl}
              helperText={errors.repositoryUrl}
              fullWidth
            />
          </Form.ElementWrapper>
          {isPrivateRepoEnabled && (
            <GitSecretSelector
              formData={formData}
              handleFieldChange={handleFieldChange}
              errors={errors}
            />
          )}
          <Form.Stack direction="row" spacing={2}>
            <Form.ElementWrapper label="Branch" name="branch">
              <TextField
                id="branch"
                placeholder="main"
                value={formData.branch}
                onChange={(e) => handleFieldChange('branch', e.target.value)}
                error={!!errors.branch}
                helperText={errors.branch}
                fullWidth
              />
            </Form.ElementWrapper>
            <Form.ElementWrapper label="Project Path" name="appPath">
              <TextField
                id="appPath"
                placeholder="my-agent"
                value={formData.appPath}
                onChange={(e) => handleFieldChange('appPath', e.target.value)}
                error={!!errors.appPath}
                helperText={errors.appPath}
                fullWidth
              />
            </Form.ElementWrapper>
          </Form.Stack>
        </Form.Stack>
      </Form.Section>

      <Form.Section>
        <Form.Subheader>Build Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.Stack direction="row" spacing={2}>
            {
              languageOptions.map((type) => {
                const isSelected = formData.language === type.value;
                return (
                  <Form.CardButton
                    key={type.value}
                    onClick={() => handleFieldChange('language', type.value)}
                    selected={isSelected}
                  >
                    <Form.CardHeader title={<Form.Stack direction="row" spacing={2} justifyContent="center" alignItems="center">
                      <BuildpackIcon language={type.value} />
                      <Form.Body>{type.label}</Form.Body>
                    </Form.Stack>} />
                  </Form.CardButton>
                );

              })
            }
          </Form.Stack>


          <Collapse in={formData.language === "python"}>
            <Form.Stack direction="row" spacing={2}>
              <Form.ElementWrapper label="Start Command" name="runCommand">
                <TextField
                  id="runCommand"
                  placeholder="python main.py"
                  value={formData.runCommand}
                  onChange={(e) => handleFieldChange('runCommand', e.target.value)}
                  error={!!errors.runCommand}
                  helperText={
                    errors.runCommand ||
                    "Dependencies auto-install from package.json, requirements.txt, or pyproject.toml"
                  }
                  fullWidth
                />
              </Form.ElementWrapper>
              <Form.ElementWrapper label="Language Version" name="languageVersion">
                <FormControl fullWidth error={!!errors.languageVersion}>
                  <Select
                    id="languageVersion"
                    value={formData.languageVersion || ''}
                    onChange={(e) => handleFieldChange('languageVersion', e.target.value)}
                    disabled={!buildOptions}
                  >
                    {(buildOptions?.python.supportedVersions ?? []).map((v) => (
                      <MenuItem key={v} value={v}>
                        {v}
                      </MenuItem>
                    ))}
                  </Select>
                  <FormHelperText>
                    {errors.languageVersion ||
                      (buildOptionsQuery.isError
                        ? 'Could not load supported Python versions; retry by refreshing.'
                        : 'Python runtime version')}
                  </FormHelperText>
                </FormControl>
              </Form.ElementWrapper>
            </Form.Stack>
            <FormControlLabel
              control={
                <Checkbox
                  checked={formData.enableAutoInstrumentation ?? true}
                  onChange={(e) => handleFieldChange('enableAutoInstrumentation', e.target.checked)}
                />
              }
              label="Enable auto instrumentation"
            />
            <Collapse in={formData.enableAutoInstrumentation !== false}>
              <Stack spacing={1}>
                <Typography variant="body2" color="text.secondary">
                  Automatically adds OTEL tracing instrumentation to your agent for observability.
                </Typography>
                {compatibleInstrumentation.length === 0 && buildOptions ? (
                  <Alert severity="info" sx={{ mt: 1 }}>
                    No AMP-provided instrumentation is available for Python{' '}
                    {formData.languageVersion ?? 'the selected version'}. You
                    can still deploy this agent and instrument it manually.
                  </Alert>
                ) : (
                  <Form.ElementWrapper
                    label="AMP Instrumentation Version"
                    name="instrumentationVersion"
                  >
                    <FormControl
                      sx={{ minWidth: 200 }}
                      error={!!errors.instrumentationVersion}
                    >
                      <Select
                        id="instrumentationVersion"
                        value={formData.instrumentationVersion || ''}
                        onChange={(e) =>
                          handleFieldChange('instrumentationVersion', e.target.value)
                        }
                        disabled={!buildOptions}
                      >
                        {compatibleInstrumentation.map((v) => (
                          <MenuItem key={v.version} value={v.version}>
                            {v.version}
                          </MenuItem>
                        ))}
                      </Select>
                      <FormHelperText>
                        {errors.instrumentationVersion ||
                          'Pins the init-container image and the bundled OpenLLMetry SDK version.'}
                      </FormHelperText>
                    </FormControl>
                  </Form.ElementWrapper>
                )}
              </Stack>
            </Collapse>
            <Collapse in={formData.enableAutoInstrumentation === false}>
              <Alert severity="info" sx={{ mt: 1 }}>
                <Typography variant="subtitle2">
                  Tracing Support for Python Agents
                </Typography>
                <Typography variant="body2" sx={{ mt: 1 }}>
                  With auto-instrumentation disabled, you can still manually instrument your Python agent using{' '}
                  your desired instrumentation library. Emit your own spans against the{' '}
                  <Typography
                    component="a"
                    href={MANUAL_INSTRUMENTATION_DOCS_URL}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ color: 'primary.main' }}
                  >
                    manual instrumentation contract
                  </Typography>
                  {' '}—{' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    amp-instrumentation
                  </Typography>
                  {' '}ships an{' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    init_otel()
                  </Typography>
                  {' '}helper that configures the OTLP exporter for you.
                </Typography>
                <Typography variant="body2" sx={{ mt: 1 }}>
                  Environment variables provided:{' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    AMP_OTEL_ENDPOINT
                  </Typography>
                  {', '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    AMP_AGENT_API_KEY
                  </Typography>
                </Typography>
                <Typography variant="body2" sx={{ mt: 1 }}>
                  Example configuration:
                </Typography>
                <Typography variant="body2" component="div" sx={{ mt: 0.5, ml: 1 }}>
                  • OTLP exporter endpoint ={' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    AMP_OTEL_ENDPOINT
                  </Typography>
                </Typography>
                <Typography variant="body2" component="div" sx={{ ml: 1 }}>
                  • OTLP headers ={' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    {'{"x-amp-api-key": AMP_AGENT_API_KEY}'}
                  </Typography>
                </Typography>
              </Alert>
            </Collapse>
          </Collapse>



          <Collapse in={formData.language === "docker"}>
            <Stack spacing={2}>
              <Form.Stack direction="row" spacing={2}>
                <Form.ElementWrapper label="Dockerfile Path" name="dockerfilePath">
                  <TextField
                    id="dockerfilePath"
                    placeholder="e.g., ./Dockerfile"
                    value={formData.dockerfilePath || ''}
                    onChange={(e) => handleFieldChange('dockerfilePath', e.target.value)}
                    error={!!errors.dockerfilePath}
                    helperText={
                      errors.dockerfilePath ||
                      "Path to Dockerfile in your repository"
                    }
                    fullWidth
                  />
                </Form.ElementWrapper>
              </Form.Stack>
              <Alert severity="info">
                <Typography variant="subtitle2" gutterBottom>
                  Tracing Support for Docker-Based Agents
                </Typography>
                <Typography variant="body2" paragraph>
                  Docker-based agents require OTEL instrumentation to export traces.
                  For Python, use{' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    {`pip install amp-instrumentation==${formData.instrumentationVersion ?? buildOptions?.instrumentation.defaultVersion ?? 'latest'}`}
                  </Typography>
                  {' '}and run with{' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    amp-instrument python your_script.py
                  </Typography>
                  {' '}for zero-code tracing. To pick a different version, see the{' '}
                  <Typography
                    component="a"
                    href={VERSION_MAPPING_DOCS_URL}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ color: 'primary.main' }}
                  >
                    AMP instrumentation version mapping
                  </Typography>
                  . For agents on a framework the Traceloop SDK doesn&apos;t cover,
                  emit your own spans against the{' '}
                  <Typography
                    component="a"
                    href={MANUAL_INSTRUMENTATION_DOCS_URL}
                    target="_blank"
                    rel="noopener noreferrer"
                    sx={{ color: 'primary.main' }}
                  >
                    manual instrumentation contract
                  </Typography>
                  .
                </Typography>
                <Typography variant="body2" gutterBottom>
                  Environment variables provided:{' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    AMP_OTEL_ENDPOINT
                  </Typography>
                  {', '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    AMP_AGENT_API_KEY
                  </Typography>
                </Typography>
                <Typography variant="body2" sx={{ mt: 1 }}>
                  Example configuration:
                </Typography>
                <Typography variant="body2" component="div" sx={{ mt: 0.5, ml: 1 }}>
                  • OTLP exporter endpoint ={' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    AMP_OTEL_ENDPOINT
                  </Typography>
                </Typography>
                <Typography variant="body2" component="div" sx={{ ml: 1 }}>
                  • OTLP headers ={' '}
                  <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                    {'{"x-amp-api-key": AMP_AGENT_API_KEY}'}
                  </Typography>
                </Typography>
              </Alert>
            </Stack>
          </Collapse>

        </Form.Stack>
      </Form.Section>

      <InputInterface
        formData={formData}
        setFormData={setFormData}
        errors={errors}
        setFieldError={setFieldError}
        validateField={validateField}
      />
      <LLMProviderSection
        llmProviders={llmProviders}
        setLLMProviders={setLLMProviders}
        agentDisplayName={formData.displayName}
        externalEnvKeys={
          new Set((formData.env ?? []).map((e) => e.key).filter((k): k is string => !!k))
        }
      />
      <EnvironmentVariable
        formData={formData}
        setFormData={setFormData}
        llmReservedNames={(() => {
          const agentNameUpper = formData.displayName
            ? formData.displayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
            : "AGENT";
          return new Set(
            llmProviders.flatMap((e, i) => [
              e.urlVarName ?? `${agentNameUpper}_${i + 1}_URL`,
              e.apikeyVarName ?? `${agentNameUpper}_${i + 1}_API_KEY`,
            ]),
          );
        })()}
      />
      <FileMount
        formData={formData}
        setFormData={setFormData}
      />
    </Form.Stack>
  );
};

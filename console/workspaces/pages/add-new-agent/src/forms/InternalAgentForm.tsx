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

import { Alert, Box, Checkbox, Collapse, Form, FormControl, FormControlLabel, FormHelperText, IconButton, MenuItem, Select, Stack, TextField, Tooltip, Typography, CircularProgress } from "@wso2/oxygen-ui";
import { Copy as ContentCopy } from "@wso2/oxygen-ui-icons-react";
import { useEffect, useMemo, useCallback, useState, type ComponentType } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import {
  useAgentBuildOptions,
  useGenerateResourceName,
} from "@agent-management-platform/api-client";
import { globalConfig, INPUT_LIMITS } from "@agent-management-platform/types";
import { InputInterface } from "../components/InputInterface";
import { EnvironmentVariable } from "../components/EnvironmentVariable";
import { FileMount } from "../components/FileMount";
import { GitSecretSelector } from "../components/GitSecretSelector";
import { LLMProviderSection } from "../components/LLMProviderSection";
import { MCPProxySection } from "../components/MCPProxySection";
import { LabelsEditor, MarkdownEditor } from "@agent-management-platform/shared-component";
import type { CreateAgentFormValues, LLMProviderFormEntry, MCPProxyFormEntry } from "../form/schema";
import {
  BuildpackIcon,
  useExternalComponentModulesByMountPoint,
  type PrivateRepoSourceProps,
} from "@agent-management-platform/views";

// Mount point where a deployment can inject its own repository-source component
// (e.g. a GitHub App connect flow). Kept as a string literal to avoid a core-ui
// dependency from this page workspace; matches MountPoints.PrivateRepoSource.
const PRIVATE_REPO_SOURCE_MOUNT_POINT = "private-repo-source";
import { mcpEntryVarNames } from "../utils/mcpEnvVarNames";

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
  mcpProxies: MCPProxyFormEntry[];
  setMCPProxies: React.Dispatch<React.SetStateAction<MCPProxyFormEntry[]>>;
  initialEnvironmentName: string | undefined;
  isInitialEnvironmentLoading?: boolean;
  // When the deployment pipeline has more than one environment, this carries
  // the first env name so each section can warn that create-time config
  // applies only to that environment.
  firstEnvOnlyNotice?: string | undefined;
}
const languageOptions = [
  { label: "Python", value: "python" },
  { label: "Ballerina", value: "ballerina" },
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
  mcpProxies,
  setMCPProxies,
  initialEnvironmentName,
  isInitialEnvironmentLoading = false,
  firstEnvOnlyNotice,
}: InternalAgentFormProps) => {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();

  const [copied, setCopied] = useState(false);
  const handleCopyBalImport = useCallback(async () => {
    try {
      await navigator.clipboard.writeText("import ballerinax/amp as _;");
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Failed to copy - silently fail
    }
  }, []);

  const isPrivateRepoEnabled = globalConfig.featureFlags?.enablePrivateRepoSupport === true;

  // A deployment can override the entire repository-source region (e.g. to connect
  // private repos through a GitHub App instead of a stored PAT). When nothing is
  // injected, the built-in fields + PAT git-secret selector render unchanged.
  const RepoSourceOverride = useExternalComponentModulesByMountPoint(
    PRIVATE_REPO_SOURCE_MOUNT_POINT,
  )[0]?.component as unknown as ComponentType<PrivateRepoSourceProps> | undefined;

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
    if (!buildOptions)
      return [] as {
        version: string;
        traceloopSdk?: string;
        pythonVersions: string[];
      }[];
    const py = formData.languageVersion;
    if (!py) return buildOptions.instrumentation.versions;
    return buildOptions.instrumentation.versions.filter((v) =>
      v.pythonVersions.includes(py),
    );
  }, [buildOptions, formData.languageVersion]);

  // Version suffix for the sample `pip install` command. Falls back to an
  // unpinned install rather than a placeholder when build options haven't
  // resolved — `amp-instrumentation==latest` is not a valid pip specifier.
  const pipVersionSpecifier = useMemo(() => {
    const version =
      formData.instrumentationVersion ?? buildOptions?.instrumentation.defaultVersion;
    return version ? `==${version}` : '';
  }, [formData.instrumentationVersion, buildOptions]);

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
          } else if (value === 'ballerina') {
            // Switching to Ballerina - no conditional fields are required, so clear
            // any stale Python/Docker errors from the now-hidden fields.
            setFieldError('runCommand', undefined);
            setFieldError('languageVersion', undefined);
            setFieldError('dockerfilePath', undefined);
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
              slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
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
            <MarkdownEditor
              id="description"
              placeholder="Short description of what this agent does. Markdown is supported."
              value={formData.description || ''}
              onChange={(value) => handleFieldChange('description', value)}
              error={!!errors.description}
              helperText={errors.description}
            />
          </Form.ElementWrapper>
          <Form.ElementWrapper label="Labels (optional)" name="labels">
            <LabelsEditor
              hideTitle
              description="Attach key/value labels to organize and filter agents."
              value={formData.labels ?? {}}
              onChange={(labels) => handleFieldChange("labels", labels)}
            />
          </Form.ElementWrapper>
        </Form.Stack>
      </Form.Section>

      <Form.Section>
        <Form.Subheader>Repository Details</Form.Subheader>
        <Form.Stack spacing={2}>
          {RepoSourceOverride ? (
            <RepoSourceOverride
              values={{
                repositoryUrl: formData.repositoryUrl,
                branch: formData.branch,
                appPath: formData.appPath,
                gitSecretRef: formData.gitSecretRef,
              }}
              errors={{
                repositoryUrl: errors.repositoryUrl,
                branch: errors.branch,
                appPath: errors.appPath,
                gitSecretRef: errors.gitSecretRef,
              }}
              onFieldChange={(field, value) => handleFieldChange(field, value)}
              sourceBinding={formData.githubApp}
              onSourceBindingChange={(binding) =>
                setFormData((previous) => ({ ...previous, githubApp: binding }))
              }
              orgId={orgId ?? ""}
              projectName={projectId}
              componentName={formData.name}
            />
          ) : (
            <>
              <Form.ElementWrapper label="GitHub Repository" name="repositoryUrl">
                <TextField
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.URL } }}
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
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.SHORT_TEXT } }}
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
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.PATH } }}
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
            </>
          )}
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
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.VALUE } }}
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
                            {v.traceloopSdk
                              ? `${v.version} (OpenLLMetry v${v.traceloopSdk})`
                              : v.version}
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



          <Collapse in={formData.language === "ballerina"}>
            <Stack spacing={2}>
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
                <Alert severity="info" sx={{ mt: 1 }}>
                  <Typography variant="subtitle2" gutterBottom>
                    Tracing Support for Ballerina-Based Agents
                  </Typography>
                  <Typography variant="body2" gutterBottom>
                    To enable the AMP extension in a Ballerina program, add the following
                    import to your program.
                  </Typography>
                  <Box sx={{ position: 'relative', mt: 1 }}>
                    <Typography
                      component="pre"
                      variant="body2"
                      sx={{ m: 0, bgcolor: 'action.hover', pl: 1, pr: 5, py: 0.5, borderRadius: 0.5, fontFamily: 'monospace' }}
                    >
                      import ballerinax/amp as _;
                    </Typography>
                    <Tooltip title={copied ? 'Copied!' : 'Copy code'}>
                      <IconButton
                        onClick={handleCopyBalImport}
                        size="small"
                        sx={{ position: 'absolute', right: 4, top: '50%', transform: 'translateY(-50%)' }}
                      >
                        <ContentCopy size={16} />
                      </IconButton>
                    </Tooltip>
                  </Box>
                  <Typography variant="body2" sx={{ mt: 1 }}>
                    Compatible with Ballerina version{' '}
                    <Typography component="code" sx={{ bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}>
                      2201.13.x +
                    </Typography>
                  </Typography>
                </Alert>
              </Collapse>
            </Stack>
          </Collapse>

          <Collapse in={formData.language === "docker"}>
            <Stack spacing={2}>
              <Form.Stack direction="row" spacing={2}>
                <Form.ElementWrapper label="Dockerfile Path" name="dockerfilePath">
                  <TextField
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.PATH } }}
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
                    {`pip install amp-instrumentation${pipVersionSpecifier}`}
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
      {firstEnvOnlyNotice && (
        <Alert severity="info">
          LLM providers, environment variables, and file mounts below apply only
          to the <strong>{firstEnvOnlyNotice}</strong> environment. Configure
          values for other environments when promoting.
        </Alert>
      )}
      <LLMProviderSection
        llmProviders={llmProviders}
        setLLMProviders={setLLMProviders}
        agentDisplayName={formData.displayName}
        initialEnvironmentName={initialEnvironmentName}
        isInitialEnvironmentLoading={isInitialEnvironmentLoading}
        externalEnvKeys={(() => {
          const agentNameUpper = formData.displayName
            ? formData.displayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
            : "AGENT";
          return new Set([
            ...(formData.env ?? []).map((e) => e.key).filter((k): k is string => !!k),
            ...mcpProxies.flatMap((e, i) => mcpEntryVarNames(e, i, agentNameUpper)),
          ]);
        })()}
      />
      <MCPProxySection
        mcpProxies={mcpProxies}
        setMCPProxies={setMCPProxies}
        agentDisplayName={formData.displayName}
        initialEnvironmentName={initialEnvironmentName}
        isInitialEnvironmentLoading={isInitialEnvironmentLoading}
        externalEnvKeys={(() => {
          const agentNameUpper = formData.displayName
            ? formData.displayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
            : "AGENT";
          return new Set([
            ...(formData.env ?? []).map((e) => e.key).filter((k): k is string => !!k),
            ...llmProviders.flatMap((e, i) => [
              e.urlVarName ?? `${agentNameUpper}_${i + 1}_URL`,
              e.apikeyVarName ?? `${agentNameUpper}_${i + 1}_API_KEY`,
            ]),
          ]);
        })()}
      />
      <EnvironmentVariable
        formData={formData}
        setFormData={setFormData}
        llmReservedNames={(() => {
          const agentNameUpper = formData.displayName
            ? formData.displayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
            : "AGENT";
          return new Set([
            ...llmProviders.flatMap((e, i) => [
              e.urlVarName ?? `${agentNameUpper}_${i + 1}_URL`,
              e.apikeyVarName ?? `${agentNameUpper}_${i + 1}_API_KEY`,
            ]),
            ...mcpProxies.flatMap((e, i) => mcpEntryVarNames(e, i, agentNameUpper)),
          ]);
        })()}
        showMissingValueError={!!errors.env}
      />
      <FileMount
        formData={formData}
        setFormData={setFormData}
      />
    </Form.Stack>
  );
};

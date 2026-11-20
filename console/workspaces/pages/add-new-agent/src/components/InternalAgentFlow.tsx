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

import React, { useCallback, useMemo, useState } from "react";
import { Alert, Form, Link } from "@wso2/oxygen-ui";
import { PageLayout, useFormValidation, useSnackBar } from "@agent-management-platform/views";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import { generatePath, useLocation, useNavigate, useParams } from "react-router-dom";
import { useAgentWizardReturn } from "../utils/useAgentWizardReturn";
import {
  absoluteRouteMap,
  type AgentSourceSeed,
  OrgProjPathParams,
} from "@agent-management-platform/types";
import {
  useAgentBuildOptions,
  useCreateAgent,
  useGetDeploymentPipeline,
  useOnboardShadowFinding,
} from "@agent-management-platform/api-client";
import { createAgentSchema, type CreateAgentFormValues, type LLMProviderFormEntry, type MCPProxyFormEntry } from "../form/schema";
import { InternalAgentForm } from "../forms/InternalAgentForm";
import { CreateButtons } from "./CreateButtons";
import {
  buildAgentCreationPayload,
  deriveSampleAgentSeed,
  agentSourceSeedValues,
  readAgentSourceSeed,
  findLowestEnvironmentName,
  hasMultipleEnvironments,
} from "../utils/buildAgentPayload";
import {
  hasUnresolvedMCPSecurity,
  mcpEntryVarNames,
} from "../utils/mcpEnvVarNames";
import {
  getSampleAgentById,
  getSampleReadmeUrl,
  type SampleAgentDefinition,
} from "../data/sampleAgents";

const buildInitialFormValues = (
  sample: SampleAgentDefinition | undefined,
  seed: AgentSourceSeed | undefined,
): CreateAgentFormValues => {
  const defaults: CreateAgentFormValues = {
    deploymentType: "new" as const,
    enableAutoInstrumentation: true,
    // instrumentationVersion and languageVersion start undefined and get
    // populated by the form from the fetched agent-build-options.
    instrumentationVersion: undefined,
    name: "",
    displayName: "",
    description: "",
    repositoryUrl: "",
    branch: "main",
    appPath: "/",
    runCommand: "python main.py",
    language: "python",
    languageVersion: undefined,
    dockerfilePath: "/Dockerfile",
    interfaceType: "DEFAULT" as const,
    port: "" as unknown as number,
    basePath: "/",
    openApiPath: "",
    env: [],
    files: [],
  };

  if (sample) return { ...defaults, ...deriveSampleAgentSeed(sample) };
  if (seed) return { ...defaults, ...agentSourceSeedValues(seed) };
  return defaults;
};

export const InternalAgentFlow: React.FC = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const { pushSnackBar } = useSnackBar();
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId?: string;
  }>();

  // Resolved once on mount from the sample picked on the previous screen, if any.
  const [selectedSample] = useState(() =>
    getSampleAgentById(new URLSearchParams(window.location.search).get("sample"))
  );
  // A code agent found by Shadow Agent Discovery, passed in router state, if any.
  const [sourceSeed] = useState(() =>
    selectedSample ? undefined : readAgentSourceSeed(location.state, orgId)
  );
  const [initialFormData] = useState<CreateAgentFormValues>(() =>
    buildInitialFormValues(selectedSample, sourceSeed)
  );
  const [formData, setFormData] = useState<CreateAgentFormValues>(initialFormData);

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<CreateAgentFormValues>(createAgentSchema);

  const [llmProviders, setLLMProviders] = useState<LLMProviderFormEntry[]>([]);
  const [mcpProxies, setMCPProxies] = useState<MCPProxyFormEntry[]>([]);

  const { mutate: createAgent, isPending, error } = useCreateAgent();
  const { mutate: linkFinding, isPending: isLinking } = useOnboardShadowFinding(
    sourceSeed?.shadowFinding?.orgName,
    sourceSeed?.shadowFinding?.findingId,
    { silent: true },
  );

  // InternalAgentForm seeds languageVersion / instrumentationVersion from the
  // build options once they load. Mirror that seeding here (same cached
  // query) so the seeded defaults are the baseline and only a user's change
  // to either version counts as an edit.
  const { data: buildOptions } = useAgentBuildOptions({ orgName: orgId ?? "" });
  const baselineFormData = useMemo<CreateAgentFormValues>(() => {
    if (!buildOptions) return initialFormData;
    // Mirror the form: keep a supported seeded version (e.g. from a sample).
    const seeded = initialFormData.languageVersion;
    const languageVersion =
      seeded && buildOptions.python.supportedVersions.includes(seeded)
        ? seeded
        : buildOptions.python.defaultVersion;
    const defaultInstrumentation = buildOptions.instrumentation.defaultVersion;
    const compat = buildOptions.instrumentation.versions.filter((v) =>
      v.pythonVersions.includes(languageVersion),
    );
    const instrumentationVersion = compat.some((c) => c.version === defaultInstrumentation)
      ? defaultInstrumentation
      : (compat[0]?.version ?? null);
    return { ...initialFormData, languageVersion, instrumentationVersion };
  }, [buildOptions, initialFormData]);
  const isDirty = useMemo(() => {
    // Until the form has applied its seeding, an unseeded version isn't an edit.
    const withSeeded = (values: CreateAgentFormValues) => ({
      ...values,
      languageVersion: values.languageVersion ?? baselineFormData.languageVersion,
      instrumentationVersion:
        values.instrumentationVersion === undefined
          ? baselineFormData.instrumentationVersion
          : values.instrumentationVersion,
    });
    return (
      llmProviders.length > 0 ||
      mcpProxies.length > 0 ||
      // The form regenerates a seeded handle from the display name, so that
      // generated handle alone is not a user change.
      JSON.stringify(sourceSeed ? { ...withSeeded(formData), name: "" } : withSeeded(formData)) !==
        JSON.stringify(sourceSeed ? { ...baselineFormData, name: "" } : baselineFormData)
    );
  }, [formData, baselineFormData, llmProviders, mcpProxies, sourceSeed]);
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);
  const { returnTo, wizardState } = useAgentWizardReturn();

  const params = useMemo<OrgProjPathParams>(
    () => ({
      orgName: orgId ?? "default",
      projName: projectId ?? "default",
    }),
    [orgId, projectId]
  );
  const { data: deploymentPipeline, isLoading: isDeploymentPipelineLoading } =
    useGetDeploymentPipeline(params);
  const initialEnvironmentName = useMemo(
    () => findLowestEnvironmentName(deploymentPipeline?.promotionPaths),
    [deploymentPipeline?.promotionPaths],
  );
  const multipleEnvironments = useMemo(
    () => hasMultipleEnvironments(deploymentPipeline?.promotionPaths),
    [deploymentPipeline?.promotionPaths],
  );
  // When the deployment pipeline spans multiple environments, the create-time
  // config (LLM providers, env vars, file mounts) applies only to the first
  // environment. Pass the name down so each section can show that hint.
  const firstEnvOnlyNotice = multipleEnvironments ? initialEnvironmentName : undefined;

  const handleCancel = useCallback(() => {
    allowNavigation(() => navigate(returnTo));
  }, [allowNavigation, navigate, returnTo]);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] = useState<
    Record<string, string | undefined>
  >({});

  const handleDeploy = useCallback(() => {
    if (!validateForm(formData)) {
      setLastSubmittedValidationErrors(errors);
      return;
    } else {
      setLastSubmittedValidationErrors({});
    }

    if ((llmProviders.length > 0 || mcpProxies.length > 0) && !initialEnvironmentName) {
      setLastSubmittedValidationErrors({
        llmProvider: "Unable to resolve the initial deployment environment for LLM provider / MCP Server configuration.",
      });
      return;
    }

    const payload = buildAgentCreationPayload(
      formData,
      params,
      llmProviders,
      initialEnvironmentName,
      mcpProxies,
    );
    const openAgent = () =>
      allowNavigation(() => {
        // Drop the seed from this history entry, so Back does not fill the form in again.
        if (sourceSeed) {
          navigate(`${location.pathname}${location.search}`, {
            replace: true,
            state: { from: returnTo },
          });
        }
        navigate(
          generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents.path,
            {
              orgId: params.orgName ?? "",
              projectId: params.projName ?? "",
              agentId: payload.body.name,
            }
          ) + "?setup=true"
        );
      });
    createAgent(payload, {
      onSuccess: () => {
        if (!sourceSeed?.shadowFinding) return openAgent();
        // Mark the found agent onboarded. The agent exists either way, so open it.
        linkFinding(
          { projectName: params.projName ?? "", agentName: payload.body.name },
          {
            onSuccess: openAgent,
            onError: (e: unknown) => {
              // 409: the found agent is already linked to an agent.
              const alreadyOnboarded = (e as { status?: number } | null)?.status === 409;
              pushSnackBar(
                alreadyOnboarded
                  ? {
                      message:
                        "The agent was created. The found agent was already onboarded as " +
                        "another agent, so it was not linked.",
                      type: "info",
                    }
                  : {
                      message: "The agent was created but could not be linked to the found agent.",
                      type: "error",
                    },
              );
              openAgent();
            },
          },
        );
      },
      onError: (e: unknown) => {
        // eslint-disable-next-line no-console
        console.error("Failed to create agent:", e);
      },
    });
  }, [
    validateForm,
    formData,
    createAgent,
    navigate,
    allowNavigation,
    params,
    errors,
    sourceSeed,
    linkFinding,
    pushSnackBar,
    location.pathname,
    location.search,
    returnTo,
    llmProviders,
    mcpProxies,
    initialEnvironmentName,
  ]);


  // A sample is picked on Add New Agent, skipping source type selection, so
  // send the user back to where they actually came from.
  const newAgentRoute = absoluteRouteMap.children.org.children.projects.children.newAgent;
  const backHref = sourceSeed
    ? returnTo
    : generatePath(
        selectedSample ? newAgentRoute.path : newAgentRoute.children.create.path,
        { orgId: orgId ?? "", projectId: projectId ?? "default" },
      );
  const backLabel = sourceSeed
    ? "Back to the found agent"
    : selectedSample
      ? "Back to add new agent"
      : "Back to source type selection";

  return (
    <PageLayout
      title="Create a Platform-Hosted Agent"
      description="Specify the source repository, select the agent type, and deploy it on the platform."
      disableIcon
      backHref={backHref}
      backState={sourceSeed ? undefined : wizardState}
      backLabel={backLabel}
    >
      <Form.Stack spacing={3}>
        {selectedSample && (
          <Alert severity="info">
            Fields are pre-filled from the <strong>{selectedSample.title}</strong> sample. See the{" "}
            <Link href={getSampleReadmeUrl(selectedSample)} target="_blank" rel="noopener noreferrer">
              README
            </Link>{" "}
            for setup and testing details.
          </Alert>
        )}
        {sourceSeed && (
          <Alert severity="info">
            Filled in from a code agent found by Shadow Agent Discovery. Check the values and fill
            in the rest, such as the run command.
          </Alert>
        )}
        <InternalAgentForm
          formData={formData}
          setFormData={setFormData}
          errors={errors}
          setFieldError={setFieldError}
          validateField={validateField}
          llmProviders={llmProviders}
          setLLMProviders={setLLMProviders}
          mcpProxies={mcpProxies}
          setMCPProxies={setMCPProxies}
          initialEnvironmentName={initialEnvironmentName}
          isInitialEnvironmentLoading={isDeploymentPipelineLoading}
          firstEnvOnlyNotice={firstEnvOnlyNotice}
        />

        {!!error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error instanceof Error ? error.message : "Failed to create agent"}
          </Alert>
        )}

        <CreateButtons
          lastSubmittedValidationErrors={lastSubmittedValidationErrors}
          isPending={
            isPending ||
            isLinking ||
            ((llmProviders.length > 0 || mcpProxies.length > 0) && isDeploymentPipelineLoading)
          }
          onCancel={handleCancel}
          onSubmit={handleDeploy}
          isNameEmpty={!formData.name.trim()}
          mode="deploy"
          hasUnresolvedMCPSecurity={hasUnresolvedMCPSecurity(mcpProxies)}
        hasLLMVarConflicts={(() => {
            const agentNameUpper = formData.displayName
              ? formData.displayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
              : "AGENT";
            const llmNames = [
              ...llmProviders.flatMap((e, i) => [
                e.urlVarName ?? `${agentNameUpper}_${i + 1}_URL`,
                e.apikeyVarName ?? `${agentNameUpper}_${i + 1}_API_KEY`,
              ]),
              ...mcpProxies.flatMap((e, i) => mcpEntryVarNames(e, i, agentNameUpper)),
            ];
            const llmNameSet = new Set(llmNames);
            // Duplicate LLM/MCP names
            if (llmNames.length !== llmNameSet.size) return true;
            // Duplicate env keys
            const envKeyList = (formData.env ?? [])
              .map((e) => e.key).filter((k): k is string => !!k);
            if (envKeyList.length !== new Set(envKeyList).size) return true;
            // Cross-conflict: env key matches an LLM/MCP name
            return envKeyList.some((k) => llmNameSet.has(k));
          })()}
        />
      </Form.Stack>
    </PageLayout>
  );
};

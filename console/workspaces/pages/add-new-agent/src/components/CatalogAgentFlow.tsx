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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Checkbox,
  Form,
  FormControlLabel,
  MenuItem,
  Select,
  SelectChangeEvent,
  Skeleton,
  Typography,
} from "@wso2/oxygen-ui";
import { PageLayout, useFormValidation } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { absoluteRouteMap, type AgentKindVersionResponse, OrgProjPathParams } from "@agent-management-platform/types";
import {
  useCreateAgent,
  useGetAgent,
  useGetAgentKind,
  useGetDeploymentPipeline,
} from "@agent-management-platform/api-client";
import { createAgentSchema, type CreateAgentFormValues, type LLMProviderFormEntry, type MCPProxyFormEntry } from "../form/schema";
import { CreateButtons } from "./CreateButtons";
import {
  buildCatalogAgentPayload,
  deriveCatalogEnvSeed,
  findLowestEnvironmentName,
  hasMultipleEnvironments,
} from "../utils/buildAgentPayload";
import { CatalogAgentForm } from "../forms/CatalogAgentForm";
import { LLMProviderSection } from "./LLMProviderSection";
import { MCPProxySection } from "./MCPProxySection";
import { EnvironmentVariable } from "./EnvironmentVariable";
import { FileMount } from "./FileMount";
import {
  hasUnresolvedMCPSecurity,
  mcpEntryVarNames,
} from "../utils/mcpEnvVarNames";

export const CatalogAgentFlow: React.FC = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const { orgId, projectId, kindId } = useParams<{
    orgId: string;
    projectId?: string;
    kindId?: string;
  }>();

  const { data: kind, isLoading: isKindLoading } = useGetAgentKind({
    orgName: orgId,
    kindName: kindId ?? "",
  });

  // Whether auto-instrumentation applies at all depends on the kind's source language,
  // which the kind itself does not carry. Read it off the source agent — which cannot be
  // deleted while the kind exists (DeleteAgent refuses with ErrAgentIsKindSource), so it
  // is always resolvable. The query stays disabled until the kind has loaded.
  const { data: sourceAgent } = useGetAgent({
    orgName: orgId,
    projName: kind?.projectName,
    agentName: kind?.agentName,
  });
  const sourceLanguage =
    sourceAgent?.build?.type === "buildpack"
      ? (sourceAgent.build as { buildpack?: { language?: string } }).buildpack?.language
      : undefined;
  // Docker-based kinds have no instrumentation trait for the flag to gate, so no toggle.
  const isInstrumentableKind =
    sourceLanguage === "python" || sourceLanguage === "ballerina";

  const sortedVersions = useMemo(
    () =>
      [...(kind?.versions ?? [])].sort(
        (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
      ),
    [kind],
  );

  const [selectedVersion, setSelectedVersion] = useState<string>("");
  const requestedVersion = searchParams.get("version") ?? "";
  const effectiveVersion = useMemo(() => {
    const preferredVersion = selectedVersion || requestedVersion;
    if (preferredVersion && sortedVersions.some((v) => v.version === preferredVersion)) {
      return preferredVersion;
    }
    return sortedVersions[0]?.version || "";
  }, [selectedVersion, requestedVersion, sortedVersions]);

  const selectedVersionData = useMemo<AgentKindVersionResponse | undefined>(
    () => kind?.versions.find((v) => v.version === effectiveVersion),
    [kind, effectiveVersion],
  );

  const [formData, setFormData] = useState<CreateAgentFormValues>({
    deploymentType: "new" as const,
    enableAutoInstrumentation: true,
    name: "",
    displayName: "",
    description: "",
    // Catalog flow intentionally hides repo/build/input type sections in UI,
    // so we seed valid defaults for those required fields.
    repositoryUrl: "https://github.com/wso2/agent-catalog-template",
    branch: "main",
    appPath: "/",
    runCommand: "python main.py",
    language: "python",
    languageVersion: "3.11",
    dockerfilePath: "/Dockerfile",
    interfaceType: "DEFAULT" as const,
    port: "" as unknown as number,
    basePath: "/",
    openApiPath: "",
    env: [],
    files: [],
  });

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<CreateAgentFormValues>(createAgentSchema);

  const catalogEnvSeed = useMemo(
    () => deriveCatalogEnvSeed(selectedVersionData?.configSchema ?? []),
    [selectedVersionData],
  );

  // Guards the reseed below on effectiveVersion itself, not on catalogEnvSeed's object
  // identity. selectedVersionData comes from a TanStack Query result that can get a new
  // reference on a background refetch (refetchOnWindowFocus is on for this query) even
  // when the same version's data is unchanged, e.g. if another version is published to
  // the same kind while this form is open. Reseeding on every such reference change
  // would silently discard whatever the user already typed into these fields.
  const seededVersionRef = useRef<string | undefined>(undefined);

  // Reseed env vars whenever the effective version actually changes, replacing whatever
  // was there before — including resetting to no rows for a version with an empty
  // schema. A schema-driven row never carries a secret's real default value (the
  // backend only signals whether one exists), so a secret field starts empty; if it
  // also has a default, the kind's own value is applied server-side at creation when
  // the field is left untouched.
  useEffect(() => {
    if (seededVersionRef.current === effectiveVersion) return;
    seededVersionRef.current = effectiveVersion;
    setFormData((prev) => ({ ...prev, env: catalogEnvSeed.env }));
  }, [catalogEnvSeed.env, effectiveVersion]);

  const lockedEnvKeys = catalogEnvSeed.lockedEnvKeys;
  const kindSecretKeys = catalogEnvSeed.kindSecretKeysWithDefault;

  const { mutate: createAgent, isPending, error } = useCreateAgent();

  const [llmProviders, setLLMProviders] = useState<LLMProviderFormEntry[]>([]);
  const [mcpProxies, setMCPProxies] = useState<MCPProxyFormEntry[]>([]);

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
  // config applies only to the first environment.
  const firstEnvOnlyNotice = multipleEnvironments ? initialEnvironmentName : undefined;

  const llmGeneratedNames = useMemo(() => {
    const agentNameUpper = formData.displayName
      ? formData.displayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
      : "AGENT";

    return [
      ...llmProviders.flatMap((entry, index) => [
        entry.urlVarName ?? `${agentNameUpper}_${index + 1}_URL`,
        entry.apikeyVarName ?? `${agentNameUpper}_${index + 1}_API_KEY`,
      ]),
      ...mcpProxies.flatMap((entry, index) =>
        mcpEntryVarNames(entry, index, agentNameUpper),
      ),
    ];
  }, [formData.displayName, llmProviders, mcpProxies]);

  const llmReservedNames = useMemo(
    () => new Set(llmGeneratedNames),
    [llmGeneratedNames],
  );

  const handleCancel = useCallback(() => {
    navigate(
      generatePath(absoluteRouteMap.children.org.children.projects.path, {
        orgId: orgId ?? "",
        projectId: projectId ?? "default",
      })
    );
  }, [navigate, orgId, projectId]);

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

    const payload = buildCatalogAgentPayload(
      formData,
      params,
      kindId ?? "",
      effectiveVersion,
      llmProviders,
      initialEnvironmentName,
      mcpProxies,
    );
    createAgent(payload, {
      onSuccess: () => {
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
      },
      onError: (e: unknown) => {
        // eslint-disable-next-line no-console
        console.error("Failed to create catalog agent:", e);
      },
    });
  }, [validateForm, formData, createAgent, navigate, params, errors, llmProviders, kindId,
    mcpProxies, effectiveVersion, initialEnvironmentName]);

  const backHref = useMemo(() => {
    return generatePath(
      absoluteRouteMap.children.org.children.projects.children.newAgent
        .children.create.children.catalog.path,
      {
      orgId: orgId ?? "",
      projectId: projectId ?? "default",
    });
  }, [orgId, projectId]);

  return (
    <PageLayout docs={["agentKindAndCatalog", "createFirstAgent", "internalAndExternalAgent"]}
      title={kind ? `Create a "${kind.displayName}" Agent` : isKindLoading ? "Loading..." : `Create a "${kindId}" Agent`}
      description="Add agent details and configure deployment settings."
      disableIcon
      backHref={backHref}
      backLabel="Back to Kind Selection"
    >
      {isKindLoading && (
        <>
          <Skeleton variant="rounded" height={32} sx={{ mb: 2, maxWidth: 320 }} />
          <Skeleton variant="rounded" height={48} sx={{ mb: 1 }} />
        </>
      )}
      <Form.Stack spacing={3}>
        <CatalogAgentForm
          formData={formData}
          setFormData={setFormData}
          errors={errors}
          setFieldError={setFieldError}
          validateField={validateField}
        />

        {sortedVersions.length > 0 && (
          <Form.Section>
            <Form.Subheader>Agent Kind Version</Form.Subheader>
            <Form.Stack spacing={2}>
              <Form.ElementWrapper
                label="Version"
                name="kindVersion"
              >
                <Select
                  size="small"
                  value={effectiveVersion}
                  onChange={(e: SelectChangeEvent<string>) => setSelectedVersion(e.target.value)}
                  sx={{ minWidth: 160 }}
                >
                  {sortedVersions.map((v) => (
                    <MenuItem key={v.version} value={v.version}>
                      v{v.version}
                    </MenuItem>
                  ))}
                </Select>
              </Form.ElementWrapper>
            </Form.Stack>
          </Form.Section>
        )}

        {/* Auto-instrumentation defaults on for every agent the platform can instrument.
            A kind whose source is already instrumented in its own code would be traced
            twice, so the choice is offered before the agent is created and auto-deployed
            rather than only afterwards. */}
        {isInstrumentableKind && (
          <Form.Section>
            <Form.Subheader>Tracing - Instrumentation</Form.Subheader>
            <Form.Stack spacing={1}>
              <FormControlLabel
                control={
                  <Checkbox
                    checked={formData.enableAutoInstrumentation ?? true}
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        enableAutoInstrumentation: e.target.checked,
                      }))
                    }
                  />
                }
                label="Enable auto instrumentation"
              />
              <Typography variant="body2" color="text.secondary">
                Adds OTEL tracing automatically. Turn this off if the agent kind is
                already instrumented in its own code, which would otherwise produce
                duplicate traces. You can change this per environment when deploying.
              </Typography>
            </Form.Stack>
          </Form.Section>
        )}

        {firstEnvOnlyNotice && (
          <Alert severity="info">
            LLM providers, environment variables, and file mounts below apply
            only to the <strong>{firstEnvOnlyNotice}</strong> environment.
            Configure values for other environments when promoting.
          </Alert>
        )}

        <LLMProviderSection
          llmProviders={llmProviders}
          setLLMProviders={setLLMProviders}
          agentDisplayName={formData.displayName}
          initialEnvironmentName={initialEnvironmentName}
          isInitialEnvironmentLoading={isDeploymentPipelineLoading}
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
          isInitialEnvironmentLoading={isDeploymentPipelineLoading}
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
          lockedKeys={lockedEnvKeys}
          kindSecretKeys={kindSecretKeys}
          resetKey={effectiveVersion}
          hideAdd
          llmReservedNames={llmReservedNames}
        />

        <FileMount
          formData={formData}
          setFormData={setFormData}
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
            ((llmProviders.length > 0 || mcpProxies.length > 0) && isDeploymentPipelineLoading)
          }
          onCancel={handleCancel}
          onSubmit={handleDeploy}
          isNameEmpty={!formData.name.trim()}
          mode="deploy"
          hasUnresolvedMCPSecurity={hasUnresolvedMCPSecurity(mcpProxies)}
        hasLLMVarConflicts={(() => {
            if (llmGeneratedNames.length !== llmReservedNames.size) return true;
            const envKeyList = (formData.env ?? [])
              .map((envEntry) => envEntry.key)
              .filter((key): key is string => !!key);
            if (envKeyList.length !== new Set(envKeyList).size) return true;
            return envKeyList.some((key) => llmReservedNames.has(key));
          })()}
        />
      </Form.Stack>
    </PageLayout>
  );
};

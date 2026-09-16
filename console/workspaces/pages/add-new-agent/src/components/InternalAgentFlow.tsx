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
import { Alert, Form } from "@wso2/oxygen-ui";
import { PageLayout, useFormValidation } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  OrgProjPathParams,
} from "@agent-management-platform/types";
import { useCreateAgent, useGetDeploymentPipeline } from "@agent-management-platform/api-client";
import { createAgentSchema, type CreateAgentFormValues, type LLMProviderFormEntry, type MCPProxyFormEntry } from "../form/schema";
import { InternalAgentForm } from "../forms/InternalAgentForm";
import { CreateButtons } from "./CreateButtons";
import {
  buildAgentCreationPayload,
  findLowestEnvironmentName,
  hasMultipleEnvironments,
} from "../utils/buildAgentPayload";
import {
  hasUnresolvedMCPSecurity,
  mcpEntryVarNames,
} from "../utils/mcpEnvVarNames";

export const InternalAgentFlow: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId?: string;
  }>();

  const [formData, setFormData] = useState<CreateAgentFormValues>({
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
  });

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<CreateAgentFormValues>(createAgentSchema);

  const [llmProviders, setLLMProviders] = useState<LLMProviderFormEntry[]>([]);
  const [mcpProxies, setMCPProxies] = useState<MCPProxyFormEntry[]>([]);

  const { mutate: createAgent, isPending, error } = useCreateAgent();

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

    const payload = buildAgentCreationPayload(
      formData,
      params,
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
        console.error("Failed to create agent:", e);
      },
    });
  }, [
    validateForm,
    formData,
    createAgent,
    navigate,
    params,
    errors,
    llmProviders,
    mcpProxies,
    initialEnvironmentName,
  ]);


  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.newAgent.children.create.path,
    { orgId: orgId ?? "", projectId: projectId ?? "default" },
  );

  return (
    <PageLayout docs={["createFirstAgent", "internalAndExternalAgent", "agentSandboxing"]}
      title="Create a Platform-Hosted Agent"
      description="Specify the source repository, select the agent type, and deploy it on the platform."
      disableIcon
      backHref={backHref}
      backLabel="Back to Source Type Selection"
    >
      <Form.Stack spacing={3}>
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

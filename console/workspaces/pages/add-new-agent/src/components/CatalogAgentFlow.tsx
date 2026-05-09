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
import { Alert, Form, MenuItem, Select, SelectChangeEvent } from "@wso2/oxygen-ui";
import { PageLayout, useFormValidation } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap, OrgProjPathParams } from "@agent-management-platform/types";
import { useCreateAgent } from "@agent-management-platform/api-client";
import { createAgentSchema, type CreateAgentFormValues, type LLMProviderFormEntry } from "../form/schema";
import { CreateButtons } from "./CreateButtons";
import { buildAgentCreationPayload } from "../utils/buildAgentPayload";
import { CatalogAgentForm } from "../forms/CatalogAgentForm";
import { LLMProviderSection } from "./LLMProviderSection";
import { DUMMY_CATALOG_LIST, getLatestVersion, type CatalogItem, type CatalogItemVersion } from "@agent-management-platform/agent-kind";
import { EnvironmentVariable } from "./EnvironmentVariable";

export const CatalogAgentFlow: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId, kindId } = useParams<{
    orgId: string;
    projectId?: string;
    kindId?: string;
  }>();

  const kindTitle = kindId
    ? kindId.replace(/-/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())
    : undefined;

  const catalogItem = useMemo(
    () => DUMMY_CATALOG_LIST.find((c: CatalogItem) => c.id === kindId),
    [kindId],
  );
  const versionKeys = useMemo(
    () =>
      catalogItem
        ? Object.entries(catalogItem.versions)
            .sort(
              ([, a], [, b]) =>
                new Date((b as CatalogItemVersion).releaseDate).getTime() - new Date((a as CatalogItemVersion).releaseDate).getTime(),
            )
            .map(([key]) => key)
        : [],
    [catalogItem],
  );
  const [selectedVersion, setSelectedVersion] = useState<string>(
    () => getLatestVersion(catalogItem!)?.versionKey ?? "",
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
  });

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<CreateAgentFormValues>(createAgentSchema);

  const [llmProviders, setLLMProviders] = useState<LLMProviderFormEntry[]>([]);

  const { mutate: createAgent, isPending, error } = useCreateAgent();

  const params = useMemo<OrgProjPathParams>(
    () => ({
      orgName: orgId ?? "default",
      projName: projectId ?? "default",
    }),
    [orgId, projectId]
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
    typeof errors
  >({});

  const handleDeploy = useCallback(() => {
    if (!validateForm(formData)) {
      setLastSubmittedValidationErrors(errors);
      return;
    } else {
      setLastSubmittedValidationErrors({});
    }

    const payload = buildAgentCreationPayload(formData, params, llmProviders);
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
  }, [validateForm, formData, createAgent, navigate, params, errors, llmProviders]);

  const backHref = useMemo(() => {
    return generatePath(absoluteRouteMap.children.org.children.projects.children.newAgent.children.create.children.catalog.path, {
      orgId: orgId ?? "",
      projectId: projectId ?? "default",
    });
  }, [orgId, projectId]);

  return (
    <PageLayout
      title={kindTitle ? `Create a "${kindTitle}" Agent` : "Create a Platform-Hosted Agent"}
      description="Add agent details and configure deployment settings."
      disableIcon
      backHref={backHref}
      backLabel="Back to Kind Selection"
    >
      <Form.Stack spacing={3}>
        <CatalogAgentForm
          formData={formData}
          setFormData={setFormData}
          errors={errors}
          setFieldError={setFieldError}
          validateField={validateField}
        />

        {versionKeys.length > 0 && (
          <Form.Section>
            <Form.Subheader>Agent Kind Version</Form.Subheader>
            <Form.Stack spacing={2}>
              <Form.ElementWrapper
                label="Version"
                name="kindVersion"
              >
                <Select
                  size="small"
                  value={selectedVersion}
                  onChange={(e: SelectChangeEvent<string>) => setSelectedVersion(e.target.value)}
                  sx={{ minWidth: 160 }}
                >
                  {versionKeys.map((key) => (
                    <MenuItem key={key} value={key}>
                      v{key}
                    </MenuItem>
                  ))}
                </Select>
              </Form.ElementWrapper>
            </Form.Stack>
          </Form.Section>
        )}

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
              llmProviders.flatMap((entry, index) => [
                entry.urlVarName ?? `${agentNameUpper}_${index + 1}_URL`,
                entry.apikeyVarName ?? `${agentNameUpper}_${index + 1}_API_KEY`,
              ]),
            );
          })()}
        />

        {!!error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error instanceof Error ? error.message : "Failed to create agent"}
          </Alert>
        )}

        <CreateButtons
          lastSubmittedValidationErrors={lastSubmittedValidationErrors}
          isPending={isPending}
          onCancel={handleCancel}
          onSubmit={handleDeploy}
          isNameEmpty={!formData.name.trim()}
          mode="deploy"
          hasLLMVarConflicts={(() => {
            const agentNameUpper = formData.displayName
              ? formData.displayName.toUpperCase().replace(/[^A-Z0-9]/g, "_")
              : "AGENT";
            const llmNames = llmProviders.flatMap((entry, index) => [
              entry.urlVarName ?? `${agentNameUpper}_${index + 1}_URL`,
              entry.apikeyVarName ?? `${agentNameUpper}_${index + 1}_API_KEY`,
            ]);
            const llmNameSet = new Set(llmNames);
            if (llmNames.length !== llmNameSet.size) return true;
            const envKeyList = (formData.env ?? [])
              .map((envEntry) => envEntry.key)
              .filter((key): key is string => !!key);
            if (envKeyList.length !== new Set(envKeyList).size) return true;
            return envKeyList.some((key) => llmNameSet.has(key));
          })()}
        />
      </Form.Stack>
    </PageLayout>
  );
};

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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Form, MenuItem, Select, SelectChangeEvent, Skeleton } from "@wso2/oxygen-ui";
import { PageLayout, useFormValidation } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap, type AgentKindVersionResponse, OrgProjPathParams } from "@agent-management-platform/types";
import { useCreateAgent, useGetAgentKind } from "@agent-management-platform/api-client";
import { createAgentSchema, type CreateAgentFormValues, type LLMProviderFormEntry } from "../form/schema";
import { CreateButtons } from "./CreateButtons";
import { buildCatalogAgentPayload } from "../utils/buildAgentPayload";
import { CatalogAgentForm } from "../forms/CatalogAgentForm";
import { LLMProviderSection } from "./LLMProviderSection";
import { EnvironmentVariable } from "./EnvironmentVariable";

export const CatalogAgentFlow: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId, kindId } = useParams<{
    orgId: string;
    projectId?: string;
    kindId?: string;
  }>();

  const { data: kind, isLoading: isKindLoading } = useGetAgentKind({
    orgName: orgId,
    kindName: kindId,
  });

  const sortedVersions = useMemo(
    () =>
      [...(kind?.versions ?? [])].sort(
        (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
      ),
    [kind],
  );

  const [selectedVersion, setSelectedVersion] = useState<string>("");
  const effectiveVersion = selectedVersion || sortedVersions[0]?.version || "";

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
  });

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<CreateAgentFormValues>(createAgentSchema);

  // Seed env vars from configSchema whenever the effective version changes
  useEffect(() => {
    const schema = selectedVersionData?.configSchema ?? [];
    if (schema.length === 0) return;
    setFormData((prev) => ({
      ...prev,
      env: schema.map((item) => ({
        key: item.name,
        value: item.defaultValue ?? "",
        isSensitive: item.isSecret,
      })),
    }));
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [effectiveVersion]);

  const lockedEnvKeys = useMemo<Set<string>>(
    () => new Set((selectedVersionData?.configSchema ?? []).map((item) => item.name)),
    [selectedVersionData],
  );

  const { mutate: createAgent, isPending, error } = useCreateAgent();

  const [llmProviders, setLLMProviders] = useState<LLMProviderFormEntry[]>([]);

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

    const payload = buildCatalogAgentPayload(formData, params, kindId ?? "", effectiveVersion, llmProviders);
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
    effectiveVersion]);

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
    <PageLayout
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
          lockedKeys={lockedEnvKeys}
          hideAdd
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

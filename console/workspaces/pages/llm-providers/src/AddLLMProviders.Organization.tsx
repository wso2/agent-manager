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

import React, { useCallback, useMemo } from "react";
import { PageLayout } from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  type CreateLLMProviderRequest,
  type UpstreamAuthType,
} from "@agent-management-platform/types";
import {
  useCreateLLMProvider,
  useListGateways,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import {
  AddLLMProviderForm,
  type AddLLMProviderFormValues,
  type GuardrailSelection,
  type TemplateCard,
} from "./subComponents/AddLLMProviderForm";

const toProviderId = (name: string): string =>
  name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

export const AddLLMProvidersOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const backHref = useMemo(
    () =>
      orgId
        ? generatePath(
            absoluteRouteMap.children.org.children.llmProviders.path,
            { orgId },
          )
        : "#",
    [orgId],
  );

  const {
    data: templatesData,
    isLoading: isLoadingTemplates,
    error: templatesError,
  } = useListLLMProviderTemplates(
    { orgName: orgId ?? "" },
    { limit: 50, offset: 0 },
  );

  const { error: gatewaysError } = useListGateways({ orgName: orgId ?? "" });

  const {
    mutate: createLLMProvider,
    isPending: isCreating,
    error: createError,
  } = useCreateLLMProvider();

  const templates: TemplateCard[] = useMemo(
    () =>
      templatesData?.templates?.map((t) => ({
        id: t.id,
        handle: t.id,
        name: t.name,
        description: t.description,
        image: t.metadata?.logoUrl,
        hasTemplateUrl: Boolean(t.metadata?.endpointUrl),
        endpointUrl: t.metadata?.endpointUrl,
        hasTemplateAuthType: Boolean(t.metadata?.auth?.type),
        hasTemplateAuthHeader: Boolean(t.metadata?.auth?.header),
        authType: t.metadata?.auth?.type,
        authHeader: t.metadata?.auth?.header,
        authValuePrefix: t.metadata?.auth?.valuePrefix,
      })) ?? [],
    [templatesData],
  );

  const missingParamsMessage = useMemo(() => {
    if (!orgId) {
      return "Organization is required to add an LLM provider.";
    }
    return null;
  }, [orgId]);

  const combinedErrorMessage = useMemo(() => {
    if (templatesError) {
      return templatesError.message;
    }
    if (gatewaysError) {
      return gatewaysError.message;
    }
    if (createError) {
      return (createError as Error)?.message || "Failed to create LLM provider";
    }
    return null;
  }, [createError, gatewaysError, templatesError]);

  const handleSubmit = useCallback(
    (values: AddLLMProviderFormValues, guardrails: GuardrailSelection[]) => {
      if (!orgId) {
        return;
      }

      const normalizedDisplayName = values.displayName?.trim() || "";
      const providerId =
        toProviderId(normalizedDisplayName) || "llm-provider";
      const selectedTemplate = templates.find(
        (tpl) => tpl.id === values.templateId,
      );
      const templateHandle =
        selectedTemplate?.handle || selectedTemplate?.name || values.templateId;

      const policies =
        guardrails.length > 0
          ? guardrails.map((g) => ({
              name: g.name,
              version: g.version,
              paths: [
                {
                  path: "/*",
                  methods: ["*"],
                  params: g.settings ?? {},
                },
              ],
            }))
          : undefined;

      const contextPath =
        values.context?.trim() || ``;

      const authType: UpstreamAuthType =
        (selectedTemplate?.authType as UpstreamAuthType) ?? "bearer";
      const authHeader =
        selectedTemplate?.authHeader ?? "Authorization";
      const apiKey = values.apiKey?.trim() ?? "";
      const authValue = apiKey
        ? (selectedTemplate?.authValuePrefix
            ? `${selectedTemplate.authValuePrefix}${apiKey}`
            : authType === "bearer"
              ? `Bearer ${apiKey}`
              : apiKey)
        : "";

      const payload: CreateLLMProviderRequest = {
        id: providerId,
        name: normalizedDisplayName || providerId,
        version: values.version.trim(),
        context: contextPath,
        template: templateHandle,
        upstream: {
          main: {
            url: values.upstreamUrl?.trim(),
            auth: values.apiKey
              ? {
                  type: authType,
                  header: authHeader,
                  value: authValue,
                }
              : undefined,
          },
        },
        description: values.description?.trim() || undefined,
        security: values.apiKey
          ? {
              enabled: true,
              apiKey: {
                enabled: true,
                key: "X-API-Key",
                in: "header",
              },
            }
          : undefined,
        policies,
        gateways:
          values.gatewayIds && values.gatewayIds.length > 0
            ? values.gatewayIds
            : undefined,
        accessControl: {
          exceptions: [],
          mode: "allow_all",
        },
      };

      createLLMProvider(
        {
          params: { orgName: orgId },
          body: payload,
        },
        {
          onSuccess: (data) => {
            const viewPath = generatePath(
              absoluteRouteMap.children.org.children.llmProviders.children.view
                .path,
              { orgId, providerId: data.uuid },
            );
            navigate(viewPath);
          },
        },
      );
    },
    [createLLMProvider, navigate, orgId, templates],
  );

  return (
    <PageLayout
      title="Add LLM Service Provider"
      backHref={backHref}
      disableIcon
      backLabel="Back to Providers List"
    >
      <AddLLMProviderForm
        templates={templates}
        isLoadingTemplates={isLoadingTemplates}
        missingParamsMessage={missingParamsMessage}
        errorMessage={combinedErrorMessage}
        isSubmitting={isCreating}
        onCancel={() => navigate(backHref)}
        onSubmit={handleSubmit}
      />
    </PageLayout>
  );
};

export default AddLLMProvidersOrganization;

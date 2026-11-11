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

import type {
  CreateLLMProviderRequest,
  LLMProviderTemplateResponse,
  UpstreamAuthType,
} from "@agent-management-platform/types";
import type {
  AddLLMProviderFormValues,
  GuardrailSelection,
  TemplateCard,
} from "../subComponents/AddLLMProviderForm";

export const toProviderId = (name: string): string =>
  name
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

export function mapLLMProviderTemplatesToCards(
  templates: LLMProviderTemplateResponse[] | undefined,
): TemplateCard[] {
  return (
    templates?.map((t) => ({
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
    })) ?? []
  );
}

export function buildCreateLLMProviderRequest(
  values: AddLLMProviderFormValues,
  guardrails: GuardrailSelection[],
  templates: TemplateCard[],
): CreateLLMProviderRequest {
  const normalizedDisplayName = values.displayName?.trim() || "";
  const providerId = toProviderId(normalizedDisplayName) || "llm-provider";
  const selectedTemplate = templates.find((tpl) => tpl.id === values.templateId);
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

  const contextPath = values.context?.trim() || ``;

  const authType: UpstreamAuthType =
    (selectedTemplate?.authType as UpstreamAuthType) ?? "api-key";
  const authHeader = selectedTemplate?.authHeader ?? "Authorization";
  const apiKey = values.apiKey?.trim() ?? "";
  const authValue = apiKey
    ? selectedTemplate?.authValuePrefix
      ? `${selectedTemplate.authValuePrefix}${apiKey}`
      : apiKey
    : "";

  return {
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
    resilience:
      values.resilienceTimeout?.trim() || values.resilienceIdleTimeout?.trim()
        ? {
            timeout: values.resilienceTimeout?.trim() || undefined,
            idleTimeout: values.resilienceIdleTimeout?.trim() || undefined,
          }
        : undefined,
    description: values.description?.trim() || undefined,
    security: values.apiKey
      ? {
          enabled: true,
          // No key: the backend resolves the ingress header itself (providerProxyAPIKeySecurity),
          // defaulting to the same header a legacy proxy uses so an agent doesn't need a
          // different header name per environment.
          apiKey: {
            enabled: true,
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
}

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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  useCreateLLMConfigAPIKey,
  useListLLMConfigAPIKeys,
  useRevokeLLMConfigAPIKey,
  useRotateLLMConfigAPIKey,
} from "@agent-management-platform/api-client";
import { DEFAULT_ENVIRONMENT } from "@agent-management-platform/types";
import { SingleAPIKeyManager } from "@agent-management-platform/shared-component";
import { useFeatureAccess } from "@agent-management-platform/feature-management";

export type LLMProxyAPIKeysSectionProps = {
  orgName: string | undefined;
  projName: string | undefined;
  agentName: string | undefined;
  /** LLM configuration UUID. */
  configId: string | undefined;
};

/**
 * Manages the single API key for an external agent's LLM configuration. The
 * key is resolved server-side from the configuration to the backing LLM proxy,
 * so the box never depends on a proxy identifier being present in the config
 * response.
 */
export function LLMProxyAPIKeysSection({
  orgName,
  projName,
  agentName,
  configId,
}: LLMProxyAPIKeysSectionProps) {
  const canManageKeys = useFeatureAccess("agent_api_key_manage");
  const { data, isLoading, isError } = useListLLMConfigAPIKeys({
    orgName,
    projName,
    agentName,
    configId,
    envName: DEFAULT_ENVIRONMENT,
  });

  const { mutateAsync: createKey, isPending: isCreating } =
    useCreateLLMConfigAPIKey();
  const { mutateAsync: rotateKey, isPending: isRotating } =
    useRotateLLMConfigAPIKey();
  const { mutate: revokeKey, isPending: isRevoking } =
    useRevokeLLMConfigAPIKey();

  const apiKey = data?.keys?.[0];

  const hasParams = !!(orgName && projName && agentName && configId);
  const keyParams = { orgName, projName, agentName, configId, envName: DEFAULT_ENVIRONMENT };
  const keyName = () =>
    `llm-config-${configId}-${Math.random().toString(36).slice(2, 10)}`;

  const handleGenerate = async () => {
    if (!hasParams) return undefined;
    const res = await createKey({
      params: keyParams,
      body: { name: keyName(), displayName: "LLM provider API key" },
    });
    return res.apiKey;
  };

  const handleRegenerate = async () => {
    if (!hasParams || !apiKey) return undefined;
    const res = await rotateKey({
      params: { ...keyParams, keyName: apiKey.name },
      body: {},
    });
    return res.apiKey;
  };

  const handleDelete = () => {
    if (!hasParams || !apiKey) return;
    revokeKey({ ...keyParams, keyName: apiKey.name });
  };

  return (
    <SingleAPIKeyManager
      scopeKey={configId ?? ""}
      description="Generate an API key to authenticate this agent's requests to the LLM provider through the gateway. Only one key can exist per configuration."
      apiKey={apiKey}
      isLoading={isLoading}
      isError={isError}
      isGenerating={isCreating}
      isRegenerating={isRotating}
      isDeleting={isRevoking}
      emptyDescription="Generate an API key to authenticate requests to this LLM provider."
      onGenerate={handleGenerate}
      onRegenerate={handleRegenerate}
      onDelete={handleDelete}
      readOnly={!canManageKeys}
    />
  );
}

export default LLMProxyAPIKeysSection;

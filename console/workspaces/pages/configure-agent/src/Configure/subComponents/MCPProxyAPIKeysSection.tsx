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
  useCreateMCPConfigAPIKey,
  useListMCPConfigAPIKeys,
  useRevokeMCPConfigAPIKey,
  useRotateMCPConfigAPIKey,
} from "@agent-management-platform/api-client";
import { SingleAPIKeyManager } from "@agent-management-platform/shared-component";
import { useResourceAccess } from "@agent-management-platform/feature-management";

export type MCPProxyAPIKeysSectionProps = {
  orgName: string | undefined;
  projName: string | undefined;
  agentName: string | undefined;
  /** MCP configuration UUID. */
  configId: string | undefined;
  /** Selected environment name. */
  envName: string | undefined;
};

/**
 * Manages the single API key for an external agent's MCP configuration in the
 * selected environment. The key is scoped to the per-config MCP mapping (the
 * credential the agent uses to call its MCP server), not the shared source proxy.
 */
export function MCPProxyAPIKeysSection({
  orgName,
  projName,
  agentName,
  configId,
  envName,
}: MCPProxyAPIKeysSectionProps) {
  const { canWrite } = useResourceAccess("agent");
  const { data, isLoading, isError } = useListMCPConfigAPIKeys({
    orgName,
    projName,
    agentName,
    configId,
    envName,
  });

  const { mutateAsync: createKey, isPending: isCreating } =
    useCreateMCPConfigAPIKey();
  const { mutateAsync: rotateKey, isPending: isRotating } =
    useRotateMCPConfigAPIKey();
  const { mutate: revokeKey, isPending: isRevoking } =
    useRevokeMCPConfigAPIKey();

  const apiKey = data?.keys?.[0];

  const hasParams = !!(orgName && projName && agentName && configId && envName);
  const keyName = () =>
    `mcp-config-${configId}-${Math.random().toString(36).slice(2, 10)}`;

  const handleGenerate = async () => {
    if (!hasParams) return undefined;
    const res = await createKey({
      params: { orgName, projName, agentName, configId, envName },
      body: { name: keyName(), displayName: "MCP server API key" },
    });
    return res.apiKey;
  };

  const handleRegenerate = async () => {
    if (!hasParams || !apiKey) return undefined;
    const res = await rotateKey({
      params: { orgName, projName, agentName, configId, envName, keyName: apiKey.name },
      body: {},
    });
    return res.apiKey;
  };

  const handleDelete = () => {
    if (!hasParams || !apiKey) return;
    revokeKey({ orgName, projName, agentName, configId, envName, keyName: apiKey.name });
  };

  return (
    <SingleAPIKeyManager
      scopeKey={`${configId ?? ""}:${envName ?? ""}`}
      description="Generate an API key to authenticate this agent's requests to the MCP server through the gateway. Only one key can exist per configuration."
      apiKey={apiKey}
      isLoading={isLoading}
      isError={isError}
      isGenerating={isCreating}
      isRegenerating={isRotating}
      isDeleting={isRevoking}
      emptyDescription="Generate an API key to authenticate requests to this MCP server."
      onGenerate={handleGenerate}
      onRegenerate={handleRegenerate}
      onDelete={handleDelete}
      readOnly={!canWrite}
    />
  );
}

export default MCPProxyAPIKeysSection;

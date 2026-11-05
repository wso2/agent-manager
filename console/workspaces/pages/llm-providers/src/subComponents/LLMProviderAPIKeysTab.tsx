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

import {
  useCreateLLMProviderAPIKey,
  useListLLMProviderAPIKeys,
  useRevokeLLMProviderAPIKey,
} from "@agent-management-platform/api-client";
import {
  APIKeysManager,
  isApiKeyAuthEnabled,
  type CreateAPIKeyInput,
} from "@agent-management-platform/shared-component";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import type { LLMProviderResponse } from "@agent-management-platform/types";
import { Alert, Button, ListingTable, Skeleton } from "@wso2/oxygen-ui";
import { ShieldOff } from "@wso2/oxygen-ui-icons-react";

export type LLMProviderAPIKeysTabProps = {
  providerData: LLMProviderResponse | null | undefined;
  orgName: string | undefined;
  providerId: string | undefined;
  isLoading?: boolean;
  /**
   * Error from the provider fetch, used to distinguish a failed load from a
   * provider with API keys disabled.
   */
  error?: Error | null;
  /** Switches the parent tab view to the Security tab. */
  onGoToSecurityTab?: () => void;
};

export function LLMProviderAPIKeysTab({
  providerData,
  orgName,
  providerId,
  isLoading = false,
  error: providerError = null,
  onGoToSecurityTab,
}: LLMProviderAPIKeysTabProps) {
  const { canWrite } = useResourceAccess("llm_provider");
  const apiKeyEnabled = isApiKeyAuthEnabled(providerData?.security);

  const {
    data,
    isLoading: isLoadingKeys,
    isError,
  } = useListLLMProviderAPIKeys({
    orgName: apiKeyEnabled ? orgName : undefined,
    providerId: apiKeyEnabled ? providerId : undefined,
  });

  const { mutateAsync: createKey, isPending: isCreating } =
    useCreateLLMProviderAPIKey();
  const { mutate: revokeKey, isPending: isRevoking } =
    useRevokeLLMProviderAPIKey();

  const handleCreate = async ({ displayName, expiresAt }: CreateAPIKeyInput) => {
    if (!orgName || !providerId) return undefined;
    const randomSuffix = Math.random().toString(36).slice(2, 10);
    const name = `provider-${providerData?.id ?? providerId}-${randomSuffix}`;
    const res = await createKey({
      params: { orgName, providerId },
      body: { name, displayName, expiresAt },
    });
    return res.apiKey;
  };

  const handleRevoke = (keyName: string) => {
    if (!orgName || !providerId) return;
    revokeKey({ orgName, providerId, keyName });
  };

  if (isLoading) {
    return <Skeleton variant="rectangular" width="100%" height={200} />;
  }

  // A failed provider fetch leaves providerData undefined; surface the error
  // explicitly instead of falling through to the "API keys disabled" message,
  // which would otherwise misrepresent the cause.
  if (providerError) {
    return (
      <Alert severity="error">
        {providerError.message || "Failed to load the LLM provider."}
      </Alert>
    );
  }

  if (!providerData) {
    return null;
  }

  if (!apiKeyEnabled) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={<ShieldOff size={48} />}
          title="API key authentication is disabled"
          description="Enable API key authentication from the Security tab to manage API keys."
          action={
            onGoToSecurityTab ? (
              <Button variant="outlined" onClick={onGoToSecurityTab}>
                Go to Security Tab
              </Button>
            ) : undefined
          }
        />
      </ListingTable.Container>
    );
  }

  return (
    <APIKeysManager
      keys={data?.keys}
      isLoading={isLoadingKeys}
      isError={isError}
      isCreating={isCreating}
      isRevoking={isRevoking}
      emptyDescription="Create an API key to authenticate requests to this LLM provider."
      onCreate={handleCreate}
      onRevoke={handleRevoke}
      readOnly={!canWrite}
    />
  );
}

export default LLMProviderAPIKeysTab;

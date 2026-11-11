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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Button, Chip, ListingTable, Tooltip } from "@wso2/oxygen-ui";
import { ShieldOff } from "@wso2/oxygen-ui-icons-react";
import {
  useCreateProjectMCPProxyAPIKey,
  useListProjectMCPProxyAPIKeys,
  useRevokeProjectMCPProxyAPIKey,
  useRotateProjectMCPProxyAPIKey,
} from "@agent-management-platform/api-client";
import {
  APIKeysManager,
  type CreateAPIKeyInput,
} from "@agent-management-platform/shared-component";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import type { ProjectMCPProxyEnvironment } from "@agent-management-platform/types";
import { bindingRequiresAPIKey, bindingUsesIdentity } from "./projectBinding";

const STATUS_COLOR: Record<string, "success" | "warning" | "error"> = {
  Deployed: "success",
  Deploying: "warning",
  Failed: "error",
};

/** A binding's deployment status, or why it has none. */
export function BindingStatusChip({
  env,
}: {
  env: ProjectMCPProxyEnvironment;
}) {
  if (!env.deploymentStatus) {
    return (
      <Tooltip
        title={
          env.deploymentStatusReason === "NO_GATEWAY"
            ? "The environment has no gateway yet. It deploys once one is added."
            : (env.deploymentStatusReason ?? "")
        }
      >
        <Chip label="Not deployed" size="small" variant="outlined" />
      </Tooltip>
    );
  }
  return (
    <Tooltip title={env.deploymentStatusReason ?? ""}>
      <Chip
        label={env.deploymentStatus}
        size="small"
        variant="outlined"
        color={STATUS_COLOR[env.deploymentStatus]}
      />
    </Tooltip>
  );
}

/** The binding's API keys, when it asks callers for one. */
export function BindingAPIKeysTab({
  env,
  orgName,
  projName,
  proxyId,
  onGoToSecurity,
}: {
  env: ProjectMCPProxyEnvironment;
  orgName: string | undefined;
  projName: string | undefined;
  proxyId: string;
  onGoToSecurity: () => void;
}) {
  const { canWrite } = useResourceAccess("mcp_server");
  const requiresKey = bindingRequiresAPIKey(env.security);
  const params = { orgName, projName, proxyId, envName: env.environmentName };
  const { data, isLoading, isError } = useListProjectMCPProxyAPIKeys(params, {
    enabled: requiresKey,
  });
  const { mutateAsync: createKey, isPending: isCreating } =
    useCreateProjectMCPProxyAPIKey();
  const { mutate: revokeKey, isPending: isRevoking } =
    useRevokeProjectMCPProxyAPIKey();
  const { mutateAsync: rotateKey, isPending: isRotating } =
    useRotateProjectMCPProxyAPIKey();

  if (!requiresKey) {
    return (
      <ListingTable.Container>
        <ListingTable.EmptyState
          illustration={<ShieldOff size={48} />}
          title={
            bindingUsesIdentity(env.security)
              ? "This environment uses OAuth"
              : "API key authentication is disabled"
          }
          description={
            bindingUsesIdentity(env.security)
              ? "Callers present OAuth access tokens instead of API keys. Switch to API keys from the Security tab to manage keys for this environment."
              : "Turn on API keys from the Security tab to manage keys for this environment."
          }
          action={
            <Button variant="outlined" onClick={onGoToSecurity}>
              Go to Security Tab
            </Button>
          }
        />
      </ListingTable.Container>
    );
  }

  const handleCreate = async (input: CreateAPIKeyInput) => {
    const res = await createKey({
      params,
      body: { displayName: input.displayName, expiresAt: input.expiresAt },
    });
    return res.apiKey;
  };

  return (
    <APIKeysManager
      keys={data?.keys}
      isLoading={isLoading}
      isError={isError}
      isCreating={isCreating}
      isRevoking={isRevoking}
      emptyDescription="Create an API key to call this MCP server in this environment."
      onCreate={handleCreate}
      onRevoke={(keyName) => revokeKey({ ...params, keyName })}
      // An empty body keeps the key's name and expiry; only its value changes.
      onRotate={async (keyName) =>
        (await rotateKey({ params: { ...params, keyName }, body: {} })).apiKey
      }
      isRotating={isRotating}
      readOnly={!canWrite}
    />
  );
}

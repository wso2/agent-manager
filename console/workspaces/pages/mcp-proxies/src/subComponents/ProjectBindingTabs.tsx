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

import { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Chip,
  FormControlLabel,
  ListingTable,
  MenuItem,
  Stack,
  Switch,
  Tooltip,
} from "@wso2/oxygen-ui";
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
import { TextInput } from "@agent-management-platform/views";
import type {
  APIKeyLocation,
  ProjectMCPProxyEnvironment,
  ProjectMCPProxyEnvironmentRequest,
} from "@agent-management-platform/types";
import {
  apiKeyLocation,
  apiKeyName,
  apiKeySecurity,
  bindingRequestFrom,
  bindingRequiresAPIKey,
  DEFAULT_API_KEY_HEADER,
} from "./projectBinding";

/** Saves a whole binding request for the selected environment. */
export type SaveBinding = (
  request: ProjectMCPProxyEnvironmentRequest,
) => Promise<unknown>;

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

/** Whether callers present an API key to the gateway, and where. */
export function BindingSecurityTab({
  env,
  onSave,
  isSaving,
}: {
  env: ProjectMCPProxyEnvironment;
  onSave: SaveBinding;
  isSaving: boolean;
}) {
  const { canWrite } = useResourceAccess("mcp_server");
  const [required, setRequired] = useState(bindingRequiresAPIKey(env.security));
  const [header, setHeader] = useState(apiKeyName(env.security));
  const [location, setLocation] = useState<APIKeyLocation>(
    apiKeyLocation(env.security),
  );

  useEffect(() => {
    setRequired(bindingRequiresAPIKey(env.security));
    setHeader(apiKeyName(env.security));
    setLocation(apiKeyLocation(env.security));
  }, [env]);

  const isDirty =
    required !== bindingRequiresAPIKey(env.security) ||
    (required &&
      (header.trim() !== apiKeyName(env.security) ||
        location !== apiKeyLocation(env.security)));

  const save = () =>
    onSave({
      ...bindingRequestFrom(env),
      security: apiKeySecurity(required, header.trim(), location),
    });

  return (
    <Stack spacing={3} maxWidth={560}>
      <FormControlLabel
        control={
          <Switch
            checked={required}
            disabled={!canWrite}
            onChange={(event) => setRequired(event.target.checked)}
          />
        }
        label="Require an API key"
      />
      {!required && (
        <Alert severity="warning">
          Anyone who can reach the gateway can call this MCP server in this
          environment.
        </Alert>
      )}
      {required && (
        <Stack direction="row" spacing={2} alignItems="flex-start">
          <TextInput
            label="Key name"
            placeholder={`e.g. ${DEFAULT_API_KEY_HEADER}`}
            size="small"
            value={header}
            disabled={!canWrite}
            onChange={(event) => setHeader(event.target.value)}
            helperText="The header callers send the key in."
            fullWidth
          />
          <TextInput
            select
            label="Location"
            size="small"
            value={location}
            disabled={!canWrite}
            onChange={(event) =>
              setLocation(event.target.value as APIKeyLocation)
            }
            sx={{ minWidth: 140 }}
          >
            <MenuItem value="header">Header</MenuItem>
            {/* The gateway reads the key from a header only; listed so a stored query shows. */}
            <MenuItem value="query" disabled>
              Query (not supported by the gateway)
            </MenuItem>
          </TextInput>
        </Stack>
      )}
      {canWrite && (
        <Stack direction="row">
          <Button
            variant="contained"
            disabled={!isDirty || isSaving || (required && !header.trim())}
            onClick={() => void save()}
          >
            Save
          </Button>
        </Stack>
      )}
    </Stack>
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
          title="API key authentication is disabled"
          description="Turn on API keys from the Security tab to manage keys for this environment."
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

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

import { useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import {
  useGetAgent,
  useCreateAgentAPIKey,
  useListAgentAPIKeys,
  useListAgentDeployments,
  useRevokeAgentAPIKey,
} from "@agent-management-platform/api-client";
import {
  Alert,
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Copy, Plus, Rocket, ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import {
  NoDataFound,
  PageLayout,
  TextInput,
} from "@agent-management-platform/views";

function formatDate(value?: string): string {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatDateTimeLocal(date: Date): string {
  const pad = (value: number) => value.toString().padStart(2, "0");
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hours = pad(date.getHours());
  const minutes = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hours}:${minutes}`;
}

function defaultExpiryValue(): string {
  const date = new Date();
  date.setMonth(date.getMonth() + 1);
  return formatDateTimeLocal(date);
}

export function AgentSecurityComponent() {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();

  const params = useMemo(
    () => ({ orgName: orgId, projName: projectId, agentName: agentId }),
    [agentId, orgId, projectId],
  );
  const { data: agent, isLoading: isAgentLoading } = useGetAgent(params);
  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments(params);
  const hasActiveDeployment = Object.values(deployments ?? {}).some(
    (deployment) => deployment.status === "active",
  );
  const isAPIKeySecurityEnabled =
    agent?.configurations?.enableApiKeySecurity ?? true;
  const canManageAPIKeys = hasActiveDeployment && isAPIKeySecurityEnabled;
  const apiKeyParams = useMemo(
    () =>
      canManageAPIKeys
        ? params
        : { orgName: undefined, projName: undefined, agentName: undefined },
    [canManageAPIKeys, params],
  );
  const { data, isLoading: isAPIKeysLoading } =
    useListAgentAPIKeys(apiKeyParams);
  const createKey = useCreateAgentAPIKey();
  const revokeKey = useRevokeAgentAPIKey();

  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [name, setName] = useState("");
  const [expiresAt, setExpiresAt] = useState(defaultExpiryValue);
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const apiKeys = data?.apiKeys ?? [];
  const isLoading = isAgentLoading || isDeploymentsLoading || isAPIKeysLoading;

  const handleCreate = async () => {
    const keyName = name.trim();
    if (!keyName) return;
    const response = await createKey.mutateAsync({
      params,
      body: {
        name: keyName,
        expiresAt: expiresAt ? new Date(expiresAt).toISOString() : undefined,
      },
    });
    setCreatedKey(response.apiKey ?? null);
    setName("");
    setExpiresAt(defaultExpiryValue());
    setIsCreateOpen(false);
  };

  const handleCopyCreatedKey = async () => {
    if (!createdKey) return;
    await navigator.clipboard.writeText(createdKey);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <PageLayout
      title="Inbound Security"
      disableIcon
      actions={
        <Button
          variant="contained"
          color="primary"
          startIcon={<Plus size={16} />}
          disabled={!canManageAPIKeys}
          onClick={() => {
            setCreatedKey(null);
            setExpiresAt(defaultExpiryValue());
            setIsCreateOpen(true);
          }}
        >
          Create
        </Button>
      }
      isLoading={isLoading}
    >
      {!isLoading && !hasActiveDeployment ? (
        <Box
          height="50vh"
          display="flex"
          alignItems="center"
          justifyContent="center"
        >
          <NoDataFound
            iconElement={Rocket}
            disableBackground
            message="Agent is not deployed"
            subtitle="Deploy your agent before managing API keys."
          />
        </Box>
      ) : !isLoading && !isAPIKeySecurityEnabled ? (
        <Box
          height="50vh"
          display="flex"
          alignItems="center"
          justifyContent="center"
        >
          <NoDataFound
            iconElement={ShieldCheck}
            disableBackground
            message="Enable API-key security"
            subtitle="Enable API-key security in the deployment configuration to manage API keys."
          />
        </Box>
      ) : (
        <Stack spacing={2}>
          {createdKey && (
            <Alert
              severity="success"
              action={
                <Tooltip title={copied ? "Copied" : "Copy key"}>
                  <IconButton size="small" onClick={handleCopyCreatedKey}>
                    <Copy size={16} />
                  </IconButton>
                </Tooltip>
              }
            >
              <Typography variant="body2" fontWeight={600}>
                You will only see this key once. Copy it now.
              </Typography>
              <Typography variant="body2" sx={{ wordBreak: "break-all" }}>
                {createdKey}
              </Typography>
            </Alert>
          )}

          {isLoading ? (
            <Stack spacing={1}>
              <Skeleton variant="rounded" height={56} />
              <Skeleton variant="rounded" height={56} />
            </Stack>
          ) : apiKeys.length === 0 ? (
            <Box
              height="45vh"
              display="flex"
              alignItems="center"
              justifyContent="center"
            >
              <NoDataFound
                iconElement={ShieldCheck}
                disableBackground
                message="No API keys"
                subtitle="Create an API key to call secured agent endpoints."
              />
            </Box>
          ) : (
            <TableContainer
              sx={{ border: 1, borderColor: "divider", borderRadius: 1 }}
            >
              <Table size="small" sx={{ minWidth: 760 }}>
                <TableHead>
                  <TableRow>
                    <TableCell>Name</TableCell>
                    <TableCell>Key</TableCell>
                    <TableCell>Status</TableCell>
                    <TableCell>Created</TableCell>
                    <TableCell>Expires</TableCell>
                    <TableCell align="right">Action</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {apiKeys.map((apiKey) => (
                    <TableRow key={apiKey.uuid}>
                      <TableCell>
                        <Typography variant="body2" fontWeight={600}>
                          {apiKey.name}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" fontFamily="monospace">
                          {apiKey.maskedApiKey}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Chip
                          label={apiKey.status}
                          size="small"
                          variant="outlined"
                        />
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" color="text.secondary">
                          {formatDate(apiKey.createdAt)}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" color="text.secondary">
                          {formatDate(apiKey.expiresAt)}
                        </Typography>
                      </TableCell>
                      <TableCell align="right">
                        <Button
                          variant="outlined"
                          color="error"
                          size="small"
                          disabled={revokeKey.isPending}
                          onClick={() =>
                            revokeKey.mutate({
                              ...params,
                              keyName: apiKey.name,
                            })
                          }
                        >
                          Revoke
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Stack>
      )}

      <Dialog
        open={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <ShieldCheck size={20} />
            <Typography variant="h6">Create API Key</Typography>
          </Stack>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ pt: 1 }}>
            <TextInput
              label="Name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              size="small"
              fullWidth
              disabled={createKey.isPending}
            />
            <TextInput
              label="Expiry Date"
              type="datetime-local"
              value={expiresAt}
              onChange={(event) => setExpiresAt(event.target.value)}
              size="small"
              fullWidth
              disabled={createKey.isPending}
              InputLabelProps={{ shrink: true }}
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button
            variant="outlined"
            onClick={() => setIsCreateOpen(false)}
            disabled={createKey.isPending}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleCreate}
            disabled={!name.trim() || createKey.isPending}
          >
            {createKey.isPending ? "Creating..." : "Create"}
          </Button>
        </DialogActions>
      </Dialog>
    </PageLayout>
  );
}

export default AgentSecurityComponent;

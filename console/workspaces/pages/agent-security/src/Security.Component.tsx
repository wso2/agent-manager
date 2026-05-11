/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useState } from "react";
import { useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Form,
  IconButton,
  Skeleton,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Copy,
  Key,
  Plus,
  Rocket,
  Trash2 as DeleteIcon,
} from "@wso2/oxygen-ui-icons-react";
import {
  useCreateAgentAPIKey,
  useGetAgent,
  useListAgentDeployments,
  useListAgentAPIKeys,
  useRevokeAgentAPIKey,
} from "@agent-management-platform/api-client";
import { NoDataFound, PageLayout } from "@agent-management-platform/views";
import type { AgentAPIKeyListItem } from "@agent-management-platform/types";

function CreateAPIKeyDialog({
  open,
  onClose,
  orgId,
  projectId,
  agentId,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  orgId: string;
  projectId: string;
  agentId: string;
  onCreated: (key: string) => void;
}) {
  const defaultExpiry = () => {
    const d = new Date();
    d.setMonth(d.getMonth() + 1);
    return d.toISOString().slice(0, 10);
  };
  const [displayName, setDisplayName] = useState("");
  const [expiresAt, setExpiresAt] = useState(defaultExpiry);

  const { mutate: createKey, isPending } = useCreateAgentAPIKey();

  const trimmedDisplayName = displayName.trim();
  const canSubmit = trimmedDisplayName.length > 0 && expiresAt.length > 0;

  const handleClose = () => {
    setDisplayName("");
    setExpiresAt(defaultExpiry());
    onClose();
  };

  const handleCreate = () => {
    if (!canSubmit) return;
    const expiresAtRFC3339 = `${expiresAt}T23:59:59.999Z`;
    createKey(
      {
        params: { orgName: orgId, projName: projectId, agentName: agentId },
        body: {
          displayName: trimmedDisplayName,
          expiresAt: expiresAtRFC3339,
        },
      },
      {
        onSuccess: (data) => {
          if (data.apiKey) {
            onCreated(data.apiKey);
          }
          handleClose();
        },
      },
    );
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create API Key</DialogTitle>
      <DialogContent>
        <Form.Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Display name"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
            fullWidth
            required
            size="small"
            placeholder="production key"
          />
          <TextField
            label="Expires"
            type="date"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            fullWidth
            required
            size="small"
            error={expiresAt.length === 0}
            slotProps={{ inputLabel: { shrink: true } }}
            helperText="Key expires at end of the selected day"
          />
        </Form.Stack>
      </DialogContent>
      <DialogActions>
        <Button variant="outlined" onClick={handleClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleCreate}
          disabled={isPending || !canSubmit}
          startIcon={isPending ? <CircularProgress size={16} /> : undefined}
        >
          {isPending ? "Creating..." : "Create"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function NewKeyBanner({ apiKey, onDismiss }: { apiKey: string; onDismiss: () => void }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(apiKey).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <Alert
      severity="success"
      onClose={onDismiss}
      sx={{ mb: 2, "& .MuiAlert-message": { flexGrow: 1 } }}
    >
      <Typography variant="subtitle2" sx={{ mb: 0.5 }}>
        You will only see this key once. Copy it now.
      </Typography>
      <Box display="flex" alignItems="center" gap={1}>
        <TextField
          size="small"
          fullWidth
          value={apiKey}
          slotProps={{ input: { readOnly: true } }}
        />
        <Tooltip title={copied ? "Copied!" : "Copy"}>
          <IconButton size="small" onClick={handleCopy} aria-label="Copy API key">
            <Copy size={16} />
          </IconButton>
        </Tooltip>
      </Box>
    </Alert>
  );
}

function APIKeyRow({
  apiKey,
  orgId,
  projectId,
  agentId,
}: {
  apiKey: AgentAPIKeyListItem;
  orgId: string;
  projectId: string;
  agentId: string;
}) {
  const { mutate: revokeKey, isPending } = useRevokeAgentAPIKey();

  const handleRevoke = () => {
    revokeKey({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
      keyName: apiKey.name,
    });
  };

  return (
    <Box
      display="flex"
      alignItems="center"
      justifyContent="space-between"
      px={2}
      py={1.5}
      sx={{ borderBottom: "1px solid", borderColor: "divider" }}
    >
      <Box display="flex" alignItems="center" gap={2}>
        <Key size={18} />
        <Box>
          <Typography variant="body2" fontWeight={500}>
            {apiKey.displayName || apiKey.name}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            {apiKey.maskedApiKey}
            {apiKey.expiresAt && ` · Expires ${new Date(apiKey.expiresAt).toLocaleDateString()}`}
          </Typography>
        </Box>
      </Box>
      <Box display="flex" alignItems="center" gap={1}>
        <Chip
          label={apiKey.status}
          size="small"
          color={apiKey.status === "active" ? "success" : "default"}
        />
        <Tooltip title="Revoke">
          <span>
            <IconButton
              size="small"
              color="error"
              onClick={handleRevoke}
              disabled={isPending}
              aria-label="Revoke API key"
            >
              <DeleteIcon size={16} />
            </IconButton>
          </span>
        </Tooltip>
      </Box>
    </Box>
  );
}

export const SecurityComponent: React.FC = () => {
  const { orgId, projectId, agentId } = useParams();
  const [createOpen, setCreateOpen] = useState(false);
  const [newKeyValue, setNewKeyValue] = useState<string | null>(null);

  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const { data: deployments, isLoading: isLoadingDeployments } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });

  const securityEnabled = agent?.configurations?.enableApiKeySecurity ?? true;
  const hasActiveDeployment = Object.values(deployments ?? {}).some(
    (deployment) => deployment.status === "active",
  );
  const shouldLoadKeys =
    !isLoadingAgent &&
    !isLoadingDeployments &&
    hasActiveDeployment &&
    securityEnabled;
  const {
    data: keys,
    isLoading: isLoadingKeys,
    isError,
  } = useListAgentAPIKeys({
    orgName: shouldLoadKeys ? orgId : undefined,
    projName: shouldLoadKeys ? projectId : undefined,
    agentName: shouldLoadKeys ? agentId : undefined,
  });
  const isLoading =
    isLoadingAgent || isLoadingDeployments || (shouldLoadKeys && isLoadingKeys);

  if (!isLoading && !hasActiveDeployment) {
    return (
      <PageLayout title="API Keys" disableIcon>
        <Box
          height="50vh"
          display="flex"
          justifyContent="center"
          alignItems="center"
        >
          <NoDataFound
            iconElement={Rocket}
            disableBackground
            message="Agent is not deployed"
            subtitle="Deploy your agent to manage API keys. You can deploy your agent by clicking the deploy button in the deploy tab."
          />
        </Box>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title="API Keys"
      disableIcon
      actions={
        securityEnabled && (keys?.length ?? 0) > 0 ? (
          <Button
            variant="contained"
            startIcon={<Plus size={16} />}
            onClick={() => setCreateOpen(true)}
          >
            Create
          </Button>
        ) : undefined
      }
    >
      <Box>
        {isLoading ? (
          <Skeleton variant="rectangular" width="100%" height={200} />
        ) : !securityEnabled ? (
          <Alert severity="info">
            API Key Security is disabled for this agent. To manage API keys, enable it from the{" "}
            <strong>Deployment</strong> settings and redeploy.
          </Alert>
        ) : (
          <>
            {newKeyValue && (
              <NewKeyBanner apiKey={newKeyValue} onDismiss={() => setNewKeyValue(null)} />
            )}

            {isError ? (
              <Alert severity="error">
                Failed to load API keys. Please refresh the page.
              </Alert>
            ) : keys && keys.length > 0 ? (
              <Box sx={{ border: "1px solid", borderColor: "divider", borderRadius: 1 }}>
                {keys.map((key) => (
                  <APIKeyRow
                    key={key.uuid}
                    apiKey={key}
                    orgId={orgId ?? ""}
                    projectId={projectId ?? ""}
                    agentId={agentId ?? ""}
                  />
                ))}
              </Box>
            ) : (
              <Box
                display="flex"
                flexDirection="column"
                alignItems="center"
                justifyContent="center"
                py={8}
                gap={2}
              >
                <Key size={48} />
                <Typography variant="h6">No API keys</Typography>
                <Typography variant="body2" color="text.secondary">
                  Create an API key to authenticate requests to this agent.
                </Typography>
                <Button
                  variant="contained"
                  startIcon={<Plus size={16} />}
                  onClick={() => setCreateOpen(true)}
                >
                  Create API Key
                </Button>
              </Box>
            )}
          </>
        )}
      </Box>

      <CreateAPIKeyDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        orgId={orgId ?? ""}
        projectId={projectId ?? ""}
        agentId={agentId ?? ""}
        onCreated={(key) => setNewKeyValue(key)}
      />
    </PageLayout>
  );
};

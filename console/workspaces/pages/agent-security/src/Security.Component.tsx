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
import { Key, Trash2 as DeleteIcon, Plus, Copy } from "@wso2/oxygen-ui-icons-react";
import {
  useCreateAgentAPIKey,
  useGetAgent,
  useListAgentAPIKeys,
  useRevokeAgentAPIKey,
} from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
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
  const [name, setName] = useState("");
  const [expiresAt, setExpiresAt] = useState("");

  const { mutate: createKey, isPending } = useCreateAgentAPIKey();

  const handleCreate = () => {
    createKey(
      {
        params: { orgName: orgId, projName: projectId, agentName: agentId },
        body: {
          name: name.trim() || undefined,
          expiresAt: expiresAt || undefined,
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

  const handleClose = () => {
    setName("");
    setExpiresAt("");
    onClose();
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create API Key</DialogTitle>
      <DialogContent>
        <Form.Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Name (optional)"
            value={name}
            onChange={(e) => setName(e.target.value)}
            fullWidth
            size="small"
            placeholder="my-api-key"
          />
          <TextField
            label="Expires At (optional)"
            type="datetime-local"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            fullWidth
            size="small"
            slotProps={{ inputLabel: { shrink: true } }}
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
          disabled={isPending}
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
          <IconButton size="small" onClick={handleCopy}>
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
            {apiKey.name}
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
  const { data: keys, isLoading: isLoadingKeys, isError } = useListAgentAPIKeys({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const securityEnabled = agent?.configurations?.enableApiKeySecurity ?? true;
  const isLoading = isLoadingAgent || isLoadingKeys;

  return (
    <PageLayout
      title="API Keys"
      disableIcon
      actions={
        securityEnabled ? (
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

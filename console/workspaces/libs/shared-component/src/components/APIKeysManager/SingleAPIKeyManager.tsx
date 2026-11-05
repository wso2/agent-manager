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

import { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Form,
  IconButton,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Copy,
  Key,
  Plus,
  RefreshCw,
  Trash2 as DeleteIcon,
} from "@wso2/oxygen-ui-icons-react";
import { NoDataFound } from "@agent-management-platform/views";
import type { APIKeyInfo } from "@agent-management-platform/types";

export interface SingleAPIKeyManagerProps {
  /**
   * Stable identifier for the resource this manager is bound to (e.g. a
   * config/env identifier). When it changes, the transient UI state (one-time
   * key banner and open confirm dialogs) is reset so it never carries over from
   * a previously selected resource.
   */
  scopeKey?: string;
  /** Section heading. Defaults to "API Keys". */
  title?: string;
  /** Helper copy shown under the heading. */
  description?: string;
  /** The existing key for this configuration, if any. */
  apiKey: APIKeyInfo | undefined;
  isLoading: boolean;
  isError?: boolean;
  /** True while a generate request is in flight. */
  isGenerating: boolean;
  /** True while a delete (revoke) request is in flight. */
  isDeleting?: boolean;
  /** True while a regenerate request is in flight. */
  isRegenerating?: boolean;
  /** Shown in the empty state when no key exists. */
  emptyDescription?: string;
  /**
   * When set, actions are disabled and this message is shown instead — e.g.
   * "Enable API key authentication from the Security tab to manage API keys."
   */
  disabledReason?: string;
  /** Generates a key and resolves with the one-time plaintext key (or undefined). */
  onGenerate: () => Promise<string | undefined>;
  /** Revokes and removes the existing key. */
  onDelete: () => void;
  /**
   * Revokes the existing key and generates a new one, resolving with the
   * one-time plaintext key (or undefined).
   */
  onRegenerate: () => Promise<string | undefined>;
  /** Shows the key without generate, regenerate or delete. */
  readOnly?: boolean;
}

function NewKeyBanner({
  apiKey,
  onDismiss,
}: {
  apiKey: string;
  onDismiss: () => void;
}) {
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

function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel,
  isBusy,
  onClose,
  onConfirm,
}: {
  open: boolean;
  title: string;
  message: string;
  confirmLabel: string;
  isBusy?: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <Typography variant="body2">{message}</Typography>
      </DialogContent>
      <DialogActions>
        <Button variant="outlined" onClick={onClose} disabled={isBusy}>
          Cancel
        </Button>
        <Button
          variant="contained"
          color="error"
          onClick={onConfirm}
          disabled={isBusy}
          startIcon={isBusy ? <CircularProgress size={16} /> : undefined}
        >
          {confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/**
 * Presentational manager for a single, configuration-scoped API key: shows the
 * masked existing key with regenerate/delete actions, or a generate action when
 * none exists. Only one key is allowed per configuration. The one-time plaintext
 * key is surfaced via a copy-once banner. Data and mutations are supplied by the
 * parent so the same UI backs both LLM proxy and MCP proxy configurations.
 */
export function SingleAPIKeyManager({
  scopeKey,
  title = "API Keys",
  description,
  apiKey,
  isLoading,
  isError = false,
  isGenerating,
  isDeleting = false,
  isRegenerating = false,
  emptyDescription = "Generate an API key to authenticate requests through this configuration.",
  disabledReason,
  onGenerate,
  onDelete,
  onRegenerate,
  readOnly = false,
}: SingleAPIKeyManagerProps) {
  const [newKeyValue, setNewKeyValue] = useState<string | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [regenerateOpen, setRegenerateOpen] = useState(false);

  // Reset transient UI state when the backing resource changes so a key banner
  // or open confirm dialog from a previous config/env never leaks into another.
  useEffect(() => {
    setNewKeyValue(null);
    setDeleteOpen(false);
    setRegenerateOpen(false);
  }, [scopeKey]);

  const handleGenerate = async () => {
    try {
      const key = await onGenerate();
      if (key) setNewKeyValue(key);
    } catch {
      // The error is surfaced by the mutation's notification handler.
    }
  };

  const handleRegenerate = async () => {
    try {
      const key = await onRegenerate();
      setRegenerateOpen(false);
      if (key) setNewKeyValue(key);
    } catch {
      // Keep the dialog open so the user can retry; error is surfaced by the
      // mutation's notification handler.
    }
  };

  const handleDelete = () => {
    onDelete();
    setDeleteOpen(false);
    setNewKeyValue(null);
  };

  const actionsDisabled = !!disabledReason;
  const busy = isGenerating || isDeleting || isRegenerating;

  return (
    <Form.Section>
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="flex-start"
        spacing={2}
      >
        <Stack spacing={0.5} sx={{ flex: 1, minWidth: 0 }}>
          <Form.Subheader>{title}</Form.Subheader>
          {description && (
            <Typography variant="body2" color="text.secondary">
              {description}
            </Typography>
          )}
        </Stack>
        {!readOnly && !actionsDisabled && !isLoading && !isError && !apiKey && (
          <Button
            variant="contained"
            size="small"
            startIcon={
              isGenerating ? <CircularProgress size={16} /> : <Plus size={16} />
            }
            onClick={() => void handleGenerate()}
            disabled={busy}
          >
            {isGenerating ? "Generating…" : "Generate"}
          </Button>
        )}
      </Stack>

      <Box sx={{ mt: 2 }}>
        {newKeyValue && (
          <NewKeyBanner
            apiKey={newKeyValue}
            onDismiss={() => setNewKeyValue(null)}
          />
        )}

        {isLoading ? (
          <Skeleton variant="rounded" height={72} />
        ) : disabledReason ? (
          <Alert severity="info">{disabledReason}</Alert>
        ) : isError ? (
          <Alert severity="error" icon={<AlertTriangle size={18} />}>
            Failed to load the API key. Please refresh the page.
          </Alert>
        ) : apiKey ? (
          <Card variant="outlined">
            <CardContent>
              <Stack
                direction="row"
                justifyContent="space-between"
                alignItems="center"
                spacing={2}
              >
                <Stack
                  direction="row"
                  spacing={2}
                  alignItems="center"
                  sx={{ minWidth: 0 }}
                >
                  <Key size={18} />
                  <Box sx={{ minWidth: 0 }}>
                    <Typography variant="body2" fontWeight={500} noWrap>
                      {apiKey.displayName || apiKey.name}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {apiKey.maskedApiKey}
                      {apiKey.expiresAt &&
                        ` · Expires ${new Date(
                          apiKey.expiresAt,
                        ).toLocaleDateString()}`}
                    </Typography>
                  </Box>
                </Stack>
                <Stack
                  direction="row"
                  spacing={1}
                  alignItems="center"
                  flexShrink={0}
                >
                  <Chip
                    label={apiKey.status}
                    size="small"
                    color={apiKey.status === "active" ? "success" : "default"}
                  />
                  {!readOnly && (
                  <>
                  <Button
                    variant="outlined"
                    size="small"
                    startIcon={
                      isRegenerating ? (
                        <CircularProgress size={16} />
                      ) : (
                        <RefreshCw size={16} />
                      )
                    }
                    onClick={() => setRegenerateOpen(true)}
                    disabled={busy}
                  >
                    Regenerate
                  </Button>
                  <Button
                    variant="outlined"
                    size="small"
                    color="error"
                    startIcon={
                      isDeleting ? (
                        <CircularProgress size={16} />
                      ) : (
                        <DeleteIcon size={16} />
                      )
                    }
                    onClick={() => setDeleteOpen(true)}
                    disabled={busy}
                  >
                    Delete
                  </Button>
                  </>
                  )}
                </Stack>
              </Stack>
            </CardContent>
          </Card>
        ) : (
          <Box
            sx={{
              border: "1px dashed",
              borderColor: "divider",
              borderRadius: 1,
            }}
          >
            <NoDataFound
              disableBackground
              icon={<Key size={48} />}
              message="No API key generated yet"
              subtitle={readOnly ? undefined : emptyDescription}
            />
          </Box>
        )}
      </Box>

      <ConfirmDialog
        open={deleteOpen}
        title="Delete API key?"
        message="This will revoke and permanently remove the API key. Any client using it will immediately stop being able to authenticate."
        confirmLabel={isDeleting ? "Deleting…" : "Delete"}
        isBusy={isDeleting}
        onClose={() => setDeleteOpen(false)}
        onConfirm={handleDelete}
      />
      <ConfirmDialog
        open={regenerateOpen}
        title="Regenerate API key?"
        message="This will revoke the current API key and generate a new one. The current key will immediately stop working, and the new key is shown only once."
        confirmLabel={isRegenerating ? "Regenerating…" : "Regenerate"}
        isBusy={isRegenerating}
        onClose={() => setRegenerateOpen(false)}
        onConfirm={() => void handleRegenerate()}
      />
    </Form.Section>
  );
}

export default SingleAPIKeyManager;

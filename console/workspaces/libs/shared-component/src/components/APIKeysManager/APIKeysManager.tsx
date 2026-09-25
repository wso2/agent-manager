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

import { useState, useEffect} from "react";
import {
  AdapterDateFns,
  Alert,
  Box,
  Button,
  Chip,
  DatePickers,
  Form,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Key,
  Plus,
  Trash2 as DeleteIcon,
} from "@wso2/oxygen-ui-icons-react";
import { endOfDay, format } from "date-fns";
import { DrawerContent, DrawerHeader, DrawerWrapper, TextInput } from "@agent-management-platform/views";
import { capitalize } from "../../utils/format";
import { monospaceInputSx } from "../AgentIdentityCredentials/AgentIdentityCredentials";
import { useConfirmationDialog } from "../ConfirmationDialog/ConfirmationDialogProvider";
import { type APIKeyInfo, type SecurityConfig, INPUT_LIMITS } from "@agent-management-platform/types";
import {
  ConsoleAction,
  useTrack,
} from "@agent-management-platform/api-client";

/**
 * Returns true when API-key authentication is enabled in a resource's security
 * config. Mirrors the shape persisted by the LLM provider / MCP proxy Security
 * tabs (security.apiKey.enabled === true).
 */
export function isApiKeyAuthEnabled(security?: SecurityConfig | null): boolean {
  return security?.enabled !== false && security?.apiKey?.enabled === true;
}

export interface CreateAPIKeyInput {
  displayName: string;
  /** RFC3339 expiry timestamp. */
  expiresAt: string;
}

export interface APIKeysManagerProps {
  /** The user-managed keys to display. */
  keys: APIKeyInfo[] | undefined;
  isLoading: boolean;
  isError?: boolean;
  /** True while a create request is in flight. */
  isCreating: boolean;
  /** True while a revoke request is in flight. */
  isRevoking?: boolean;
  /** Empty-state copy, e.g. "Create an API key to authenticate requests to this agent." */
  emptyDescription: string;
  /** Creates a key and resolves with the one-time plaintext key (or undefined). */
  onCreate: (input: CreateAPIKeyInput) => Promise<string | undefined>;
  /** Revokes the key with the given name. */
  onRevoke: (keyName: string) => void;
}

function CreateAPIKeyDrawer({
  open,
  onClose,
  isCreating,
  onCreate,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  isCreating: boolean;
  onCreate: (input: CreateAPIKeyInput) => Promise<string | undefined>;
  onCreated: (key: string) => void;
}) {
  const defaultExpiry = () => {
    const d = new Date();
    d.setMonth(d.getMonth() + 1);
    return d;
  };
  const [displayName, setDisplayName] = useState("");
  const [expiresAt, setExpiresAt] = useState<Date | null>(defaultExpiry);

  const trimmedDisplayName = displayName.trim();
  const isValidExpiry = !!expiresAt && !Number.isNaN(expiresAt.getTime());
  const canSubmit = trimmedDisplayName.length > 0 && isValidExpiry;

  const handleClose = () => {
    setDisplayName("");
    setExpiresAt(defaultExpiry());
    onClose();
  };

  const handleCreate = async () => {
    if (!canSubmit || !expiresAt) return;
    // Interpret the picked date as the end of that day in the user's local time
    // zone (not UTC), so the selected calendar day is preserved regardless of
    // the user's offset. toISOString() then yields the correct RFC3339 instant.
    const expiresAtRFC3339 = endOfDay(expiresAt).toISOString();
    try {
      const key = await onCreate({
        displayName: trimmedDisplayName,
        expiresAt: expiresAtRFC3339,
      });
      if (key) onCreated(key);
      handleClose();
    } catch {
      // The error is surfaced by the mutation's notification handler; keep the
      // dialog open so the user can retry.
    }
  };

  return (
    <DrawerWrapper open={open} onClose={handleClose}>
      <DrawerHeader icon={<Key size={24} />} title="Create API Key" onClose={handleClose} />
      <DrawerContent>
        <DatePickers.LocalizationProvider dateAdapter={AdapterDateFns}>
          <Stack spacing={3}>
            <Form.Section>
              <Form.Stack spacing={2}>
                <Form.ElementWrapper label="Display name" name="displayName">
                  <TextField
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                    id="displayName"
                    value={displayName}
                    onChange={(e) => setDisplayName(e.target.value)}
                    fullWidth
                    size="small"
                    placeholder="production key"
                    disabled={isCreating}
                  />
                </Form.ElementWrapper>
                <Form.ElementWrapper label="Expires" name="expiresAt">
                  <DatePickers.DatePicker
                    value={expiresAt}
                    onChange={(value) => setExpiresAt(value)}
                    minDate={new Date()}
                    disabled={isCreating}
                    slotProps={{
                      textField: {
                        size: "small",
                        fullWidth: true,
                        error: !isValidExpiry,
                        helperText: "Key expires at the end of the selected day",
                      },
                    }}
                  />
                </Form.ElementWrapper>
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1}>
              <Button variant="outlined" color="inherit" onClick={handleClose} disabled={isCreating}>
                Cancel
              </Button>
              <Button
                variant="contained"
                onClick={() => void handleCreate()}
                disabled={isCreating || !canSubmit}
              >
                {isCreating ? "Creating..." : "Create"}
              </Button>
            </Box>
          </Stack>
        </DatePickers.LocalizationProvider>
      </DrawerContent>
    </DrawerWrapper>
  );
}

function NewKeyBanner({
  apiKey,
  onDismiss,
}: {
  apiKey: string;
  onDismiss: () => void;
}) {
  const { track } = useTrack();
  // A key was put on screen. Worth counting on its own — it says the
  // issue flow completed all the way to the user actually receiving the
  // secret, which the create call alone does not. The key is never reported.
  useEffect(() => {
    track(ConsoleAction.SecretRevealed, {
      secret_type: "api-key",
      surface: "issue-banner",
    });
  }, [track]);

  return (
    <Alert
      severity="info"
      onClose={onDismiss}
      sx={{ mb: 2, "& .MuiAlert-message": { flexGrow: 1 } }}
    >
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        You will only see this key once. Copy it now.
      </Typography>
      <TextInput
        size="small"
        fullWidth
        value={apiKey}
        copyable
        copyTooltipText="Copy API key"
        slotProps={{ input: { readOnly: true } }}
        sx={monospaceInputSx}
      />
    </Alert>
  );
}

/**
 * Presentational manager for an artifact's user-managed API keys: empty state,
 * create dialog (display name + expiry), one-time key banner, key list and
 * revoke. Data and mutations are supplied by the parent so the same UI can back
 * agents, LLM providers and MCP proxies without duplicating it.
 */
export function APIKeysManager({
  keys,
  isLoading,
  isError = false,
  isCreating,
  isRevoking = false,
  emptyDescription,
  onCreate,
  onRevoke,
}: APIKeysManagerProps) {
  const [createOpen, setCreateOpen] = useState(false);
  const [newKeyValue, setNewKeyValue] = useState<string | null>(null);
  const { addConfirmation } = useConfirmationDialog();

  if (isLoading) {
    return <Skeleton variant="rectangular" width="100%" height={200} />;
  }

  const hasKeys = !!keys && keys.length > 0;

  const handleRevoke = (key: APIKeyInfo) => {
    addConfirmation({
      analytics: { entity: "api-key", action: "revoke" },
      title: "Revoke API Key",
      description: `Are you sure you want to revoke "${key.displayName || key.name}"? Any requests using this key will stop working immediately. This action cannot be undone.`,
      confirmButtonText: "Revoke",
      confirmButtonColor: "error",
      confirmButtonIcon: <DeleteIcon size={16} />,
      onConfirm: () => onRevoke(key.name),
    });
  };

  const createButton = (
    <Button
      variant="contained"
      startIcon={<Plus size={16} />}
      onClick={() => setCreateOpen(true)}
    >
      Create API Key
    </Button>
  );

  return (
    <Box>
      {newKeyValue && (
        <NewKeyBanner
          apiKey={newKeyValue}
          onDismiss={() => setNewKeyValue(null)}
        />
      )}

      {isError ? (
        <Alert severity="error">
          Failed to load API keys. Please refresh the page.
        </Alert>
      ) : hasKeys ? (
        <ListingTable.Container>
          <Box display="flex" justifyContent="flex-end" sx={{ p: 2 }}>
            {createButton}
          </Box>
          <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell width="25%">Name</ListingTable.Cell>
                <ListingTable.Cell width="25%">Key</ListingTable.Cell>
                <ListingTable.Cell width="120px">Status</ListingTable.Cell>
                <ListingTable.Cell width="180px">Expires</ListingTable.Cell>
                <ListingTable.Cell align="right" width="72px" />
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {keys!.map((key) => (
                <ListingTable.Row key={key.name} hover>
                  <ListingTable.Cell>
                    <Typography noWrap variant="body2" color="text.primary">
                      {key.displayName || key.name}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography
                      noWrap
                      variant="caption"
                      sx={{ fontFamily: "monospace", color: "text.secondary" }}
                    >
                      {key.maskedApiKey}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Chip
                      label={capitalize(key.status)}
                      size="small"
                      variant="outlined"
                      color={key.status === "active" ? "success" : "default"}
                    />
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography noWrap variant="caption" color="text.secondary">
                      {key.expiresAt
                        ? format(new Date(key.expiresAt), "dd/MM/yyyy HH:mm:ss")
                        : "Never expires"}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    <Tooltip title="Revoke">
                      <span>
                        <IconButton
                          size="small"
                          onClick={() => handleRevoke(key)}
                          disabled={isRevoking}
                          aria-label="Revoke API key"
                          sx={{ color: "text.secondary" }}
                        >
                          <DeleteIcon size={16} />
                        </IconButton>
                      </span>
                    </Tooltip>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
        </ListingTable.Container>
      ) : (
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Key size={48} />}
            title="No API keys"
            description={emptyDescription}
            action={createButton}
          />
        </ListingTable.Container>
      )}

      <CreateAPIKeyDrawer
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        isCreating={isCreating}
        onCreate={onCreate}
        onCreated={(key) => setNewKeyValue(key)}
      />
    </Box>
  );
}

export default APIKeysManager;

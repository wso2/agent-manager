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

import React, { useEffect, useState } from "react";
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Form,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useUpdateLLMProvider } from "@agent-management-platform/api-client";
import { getErrorMessage } from "@agent-management-platform/shared-component";

export interface RenameLLMProviderDialogProps {
  open: boolean;
  onClose: () => void;
  orgName: string;
  providerId: string;
  currentName: string;
}

export function RenameLLMProviderDialog({
  open,
  onClose,
  orgName,
  providerId,
  currentName,
}: RenameLLMProviderDialogProps) {
  const [name, setName] = useState(currentName);
  const [error, setError] = useState<string | null>(null);

  const { mutateAsync: updateProvider, isPending } = useUpdateLLMProvider();

  useEffect(() => {
    if (open) {
      setName(currentName);
      setError(null);
    }
  }, [open, currentName]);

  const trimmedName = name.trim();
  const isUnchanged = trimmedName === currentName.trim();

  const validate = (): string | null => {
    if (!trimmedName) {
      return "Display name is required";
    }
    if (trimmedName.length < 2) {
      return "Display name must be at least 2 characters";
    }
    if (trimmedName.length > 120) {
      return "Display name must be at most 120 characters";
    }
    return null;
  };

  const handleClose = () => {
    if (isPending) return;
    setError(null);
    onClose();
  };

  const handleSubmit = async (e?: React.FormEvent) => {
    if (e) {
      e.preventDefault();
    }
    const validationError = validate();
    if (validationError) {
      setError(validationError);
      return;
    }
    if (isUnchanged) {
      handleClose();
      return;
    }

    try {
      setError(null);
      await updateProvider({
        params: { orgName, providerId },
        body: { name: trimmedName },
      });
      onClose();
    } catch (err) {
      setError(getErrorMessage(err) || "Failed to rename LLM provider.");
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Rename LLM Provider</DialogTitle>
      <DialogContent>
        <Form.Stack spacing={3} sx={{ mt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Change the display name of this LLM provider.
          </Typography>

          {error && <Alert severity="error">{error}</Alert>}

          <Form.ElementWrapper label="Display Name" name="displayName">
            <TextField
              id="llm-provider-display-name"
              label="Display Name"
              fullWidth required
              autoFocus
              value={name}
              onChange={(e) => {
                setName(e.target.value);
                if (error) setError(null);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  void handleSubmit();
                }
              }}
              disabled={isPending}
              placeholder="e.g. OpenAI GPT-4o"
              helperText="Must be between 2 and 120 characters"
            />
          </Form.ElementWrapper>
        </Form.Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={() => void handleSubmit()}
          disabled={isPending || isUnchanged || !trimmedName}
        >
          {isPending ? "Renaming..." : "Save"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

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

import { useEffect, useId, useState, type FormEvent } from "react";
import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Form,
  OutlinedInput,
} from "@wso2/oxygen-ui";

/** A confirm dialog, with an optional phrase to type before confirming. */
export type ConfirmDialogProps = {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  destructive?: boolean;
  /** When set, Confirm stays disabled until this exact text is typed. */
  confirmPhrase?: string;
  confirmInputLabel?: string;
  confirmDisabled?: boolean;
  loading?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
};

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  destructive,
  confirmPhrase,
  confirmInputLabel,
  confirmDisabled,
  loading,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const [typed, setTyped] = useState("");
  const fieldId = useId();
  // The typed phrase is cleared whenever the dialog closes.
  useEffect(() => {
    if (!open) setTyped("");
  }, [open]);
  const matched = !confirmPhrase || typed === confirmPhrase;
  const canConfirm = matched && !confirmDisabled && !loading;
  // A real form, so Enter in the phrase field confirms.
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (canConfirm) onConfirm();
  };
  return (
    <Dialog fullWidth maxWidth="xs" onClose={onCancel} open={open}>
      <DialogTitle>{title}</DialogTitle>
      <Box component="form" noValidate onSubmit={handleSubmit}>
        <DialogContent>
          <Form.Stack spacing={2}>
            <DialogContentText>{message}</DialogContentText>
            {confirmPhrase && (
              <Form.ElementWrapper
                label={confirmInputLabel ?? `Type "${confirmPhrase}" to confirm`}
                name={fieldId}
              >
                <OutlinedInput
                  autoFocus
                  id={fieldId}
                  onChange={(event) => setTyped(event.target.value)}
                  placeholder={confirmPhrase}
                  size="small"
                  value={typed}
                />
              </Form.ElementWrapper>
            )}
          </Form.Stack>
        </DialogContent>
        <DialogActions>
          <Button color="inherit" disabled={loading} onClick={onCancel}>
            {cancelLabel}
          </Button>
          <Button
            color={destructive ? "error" : "primary"}
            disabled={!canConfirm}
            type="submit"
            variant="contained"
          >
            {confirmLabel}
          </Button>
        </DialogActions>
      </Box>
    </Dialog>
  );
}

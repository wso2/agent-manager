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

import { useEffect, useRef, useState } from "react";
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
} from "@wso2/oxygen-ui";
import { useUploadShadowInventory } from "@agent-management-platform/api-client";
import type {
  ShadowConnectionResponse,
  ShadowInventoryUploadResponse,
  ShadowProviderSpec,
} from "@agent-management-platform/types";
import {
  InventoryFilePanel,
  inventoryUploadErrorMessage,
  useInventoryFile,
} from "./InventoryFilePanel";

/** Upload file: sends a new inventory file to a Bring your own inventory source. */
export function InventoryUploadDialog({
  orgName,
  target,
  spec,
  onClose,
}: {
  orgName: string | undefined;
  /** The source to upload to; the dialog is open while set. */
  target: ShadowConnectionResponse | null;
  spec: ShadowProviderSpec | undefined;
  onClose: () => void;
}) {
  const upload = useUploadShadowInventory(orgName);
  const state = useInventoryFile();
  const [result, setResult] = useState<ShadowInventoryUploadResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { reset: resetFile } = state;
  const { reset: resetUpload } = upload;
  const open = !!target;
  // The title keeps the name while the dialog closes.
  const lastName = useRef("");
  if (target) lastName.current = target.name;

  // Every opening starts empty.
  useEffect(() => {
    if (!open) return;
    resetFile();
    resetUpload();
    setResult(null);
    setError(null);
  }, [open, resetFile, resetUpload]);

  const close = () => {
    if (!upload.isPending) onClose();
  };
  const submit = () => {
    if (!target || !state.file) return;
    setError(null);
    upload.mutate(
      { connectionId: target.id, file: state.file },
      {
        onSuccess: (r) => {
          setResult(r);
          resetUpload();
        },
        onError: (e) => setError(inventoryUploadErrorMessage(e)),
      },
    );
  };

  return (
    <Dialog open={open} onClose={close} maxWidth="md" fullWidth>
      <DialogTitle>Upload file: {target?.name ?? lastName.current}</DialogTitle>
      <DialogContent>
        <InventoryFilePanel
          orgName={orgName}
          fields={spec?.inventoryFields ?? []}
          state={state}
          result={result}
          error={error}
          disabled={upload.isPending}
        />
      </DialogContent>
      <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
        {result ? (
          <Button variant="contained" onClick={onClose}>
            Done
          </Button>
        ) : (
          <>
            <Button
              variant="contained"
              onClick={submit}
              disabled={!state.file || upload.isPending}
            >
              {upload.isPending ? "Uploading…" : "Upload"}
            </Button>
            <Button variant="outlined" onClick={close} disabled={upload.isPending}>
              Cancel
            </Button>
          </>
        )}
      </DialogActions>
    </Dialog>
  );
}

export default InventoryUploadDialog;

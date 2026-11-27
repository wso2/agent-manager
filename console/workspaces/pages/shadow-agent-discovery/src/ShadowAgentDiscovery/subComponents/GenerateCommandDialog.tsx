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
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { useCreateShadowRunnerScan } from "@agent-management-platform/api-client";
import { CodeBlock } from "@agent-management-platform/shared-component";
import type {
  ShadowConnectionResponse,
  ShadowProviderSpec,
  ShadowRunnerScanResponse,
} from "@agent-management-platform/types";
import { buildScanCommand } from "../scanCommand";
import { IssuedNotice, ScanOptions } from "./ScanCommandPanel";

/**
 * Generate command for a source code repository connection: choose
 * AI-enhanced detection and direct upload, then copy the
 * docker command that scans the repositories on the customer's machine. The
 * command is composed from the connection's saved settings and the issued
 * scan; with direct upload it holds a scan token, so it lives only in this
 * dialog's state and is shown once.
 */
export function GenerateCommandDialog({
  orgName,
  target,
  spec,
  onClose,
}: {
  orgName: string | undefined;
  /** The connection to generate for; the dialog is open while it is set. */
  target: ShadowConnectionResponse | null;
  /** The connection's provider spec (its endpoint defaults). */
  spec: ShadowProviderSpec | undefined;
  onClose: () => void;
}) {
  const { mutate, reset, isPending } = useCreateShadowRunnerScan(orgName);
  const [aiReview, setAiReview] = useState(false);
  const [directUpload, setDirectUpload] = useState(false);
  const [result, setResult] = useState<ShadowRunnerScanResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Every opening starts from a clean form, so a shown command never
  // reappears. reset() also drops a late answer to an earlier request.
  const targetId = target?.id;
  useEffect(() => {
    if (!targetId) return;
    reset();
    setAiReview(false);
    setDirectUpload(false);
    setResult(null);
    setError(null);
  }, [targetId, reset]);

  const submit = () => {
    if (!target) return;
    setError(null);
    mutate(
      { connectionId: target.id, aiReview, llmJudge: false, directUpload },
      {
        onSuccess: (r) => {
          setResult(r);
          // The command holds a token: keep it only in this dialog's state.
          reset();
        },
        onError: (e) =>
          setError(
            (e as { message?: string })?.message ??
              "The command could not be generated. Try again.",
          ),
      },
    );
  };

  return (
    <Dialog
      open={!!target}
      onClose={(_e, reason) => {
        // A shown command is lost when closed, so only Close closes it.
        if (result && (reason === "backdropClick" || reason === "escapeKeyDown")) return;
        onClose();
      }}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>Generate scan command{target ? `: ${target.name}` : ""}</DialogTitle>
      <DialogContent>
        {result && target ? (
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              Run this in a terminal. It asks for the tokens it needs, scans on your machine and
              saves the agents it confirms in a results file.
            </Typography>
            <CodeBlock
              code={buildScanCommand(
                spec ?? { provider: target.provider, endpoints: [] },
                target,
                { aiReview, directUpload },
                result,
              )}
              language="bash"
              fieldId="shadow-scan-command"
              analyticsId="shadow-scan-command"
            />
            <IssuedNotice
              issued={result}
              directUpload={directUpload}
              later="generate a new one"
            />
          </Stack>
        ) : (
          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              The rules publish an agent only when they prove it. Choose what happens to the
              matches they cannot prove.
            </Typography>
            <ScanOptions
              aiReview={aiReview}
              directUpload={directUpload}
              onChange={(next) => {
                setAiReview(next.aiReview);
                setDirectUpload(next.directUpload);
              }}
            />
            {error && <Alert severity="error">{error}</Alert>}
          </Stack>
        )}
      </DialogContent>
      <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
        {result && target ? (
          <Button variant="outlined" onClick={onClose}>
            Close
          </Button>
        ) : (
          <>
            {/* The command needs the platform's spec (its default server address). */}
            <Button variant="contained" onClick={submit} disabled={isPending || !spec}>
              Generate
            </Button>
            <Button variant="outlined" onClick={onClose} disabled={isPending}>
              Cancel
            </Button>
          </>
        )}
      </DialogActions>
    </Dialog>
  );
}

export default GenerateCommandDialog;

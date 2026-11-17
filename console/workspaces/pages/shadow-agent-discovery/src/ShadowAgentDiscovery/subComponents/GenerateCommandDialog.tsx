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
  FormControlLabel,
  Stack,
  Switch,
  Typography,
} from "@wso2/oxygen-ui";
import { useCreateShadowRunnerScan } from "@agent-management-platform/api-client";
import { CodeBlock } from "@agent-management-platform/shared-component";
import type { ShadowRunnerScanResponse } from "@agent-management-platform/types";

/** The GitHub or Azure DevOps connection a command is generated for. */
export interface CommandTarget {
  id: string;
  name: string;
}

/**
 * Generate command for a source code repository connection: choose AI review
 * (and the LLM judge), then copy the docker command that scans the
 * repositories on the customer's machine. The command holds a scan token, so
 * it lives only in this dialog's state and is shown once.
 */
export function GenerateCommandDialog({
  orgName,
  target,
  onClose,
}: {
  orgName: string | undefined;
  /** The connection to generate for; the dialog is open while it is set. */
  target: CommandTarget | null;
  onClose: () => void;
}) {
  const { mutate, reset, isPending } = useCreateShadowRunnerScan(orgName);
  const [aiReview, setAiReview] = useState(false);
  const [llmJudge, setLlmJudge] = useState(false);
  const [result, setResult] = useState<ShadowRunnerScanResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Every opening starts from a clean form, so a shown command never
  // reappears. reset() also drops a late answer to an earlier request.
  const targetId = target?.id;
  useEffect(() => {
    if (!targetId) return;
    reset();
    setAiReview(false);
    setLlmJudge(false);
    setResult(null);
    setError(null);
  }, [targetId, reset]);

  const submit = () => {
    if (!target) return;
    setError(null);
    mutate(
      { connectionId: target.id, aiReview, llmJudge: aiReview && llmJudge },
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

  const expiresAt = result ? new Date(result.expiresAt) : null;
  const expires =
    expiresAt && !Number.isNaN(expiresAt.getTime()) ? expiresAt.toLocaleTimeString() : "";

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
        {result ? (
          <Stack spacing={2}>
            <Typography variant="body2" color="text.secondary">
              Run this in a terminal. It asks for the tokens it needs, scans on your machine and
              publishes only the agents it confirms. The scan shows as Pending until then.
            </Typography>
            <CodeBlock
              code={result.command}
              language="bash"
              fieldId="shadow-scan-command"
              analyticsId="shadow-scan-command"
            />
            <Alert severity="info">
              Shown once: copy it now. It is valid for 1 hour{expires ? `, until ${expires}` : ""};
              after that, generate a new one.
            </Alert>
          </Stack>
        ) : (
          <Stack spacing={1.5}>
            <Typography variant="body2" color="text.secondary">
              The rules publish an agent only when they prove it. Choose what happens to the
              matches they cannot prove.
            </Typography>
            <FormControlLabel
              control={
                <Switch
                  checked={aiReview}
                  onChange={(e) => {
                    setAiReview(e.target.checked);
                    if (!e.target.checked) setLlmJudge(false);
                  }}
                />
              }
              label={
                <Stack>
                  <Typography variant="body2">AI review</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Your LLM checks the matches the rules cannot prove. The command asks for your
                    LLM API key.
                  </Typography>
                </Stack>
              }
            />
            <FormControlLabel
              sx={{ pl: 4 }}
              disabled={!aiReview}
              control={
                <Switch checked={llmJudge} onChange={(e) => setLlmJudge(e.target.checked)} />
              }
              label={
                <Stack>
                  <Typography variant="body2">LLM judge</Typography>
                  <Typography variant="caption" color="text.secondary">
                    A second, smaller model must agree before an agent is published. Needs AI
                    review.
                  </Typography>
                </Stack>
              }
            />
            {aiReview && <Alert severity="warning">AI review can make mistakes.</Alert>}
            {error && <Alert severity="error">{error}</Alert>}
          </Stack>
        )}
      </DialogContent>
      <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
        {result ? (
          <Button variant="outlined" onClick={onClose}>
            Close
          </Button>
        ) : (
          <>
            <Button variant="contained" onClick={submit} disabled={isPending}>
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

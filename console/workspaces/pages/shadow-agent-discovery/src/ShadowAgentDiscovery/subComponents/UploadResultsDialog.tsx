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

import { useEffect, useRef, useState, type DragEvent } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { FileUp } from "@wso2/oxygen-ui-icons-react";
import {
  isRequestTooLargeError,
  useCancelShadowScan,
  useShadowScan,
  useUploadShadowScanResults,
} from "@agent-management-platform/api-client";
import {
  formatRelativeTime,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import {
  SHADOW_SCAN_RESULTS_FORMAT,
  type ShadowScanResponse,
  type ShadowScanResultsFile,
} from "@agent-management-platform/types";
import { formatCount, SCAN_STATUS_LABELS } from "../shadowUi";

/** The largest results Agent Manager accepts (as the upload endpoint), as sent. */
export const MAX_RESULTS_BYTES = 8 * 1024 * 1024;

/**
 * The largest results file read. The file wraps the results and may be
 * pretty-printed, so it can be larger than the results it holds; those are
 * checked against MAX_RESULTS_BYTES once parsed.
 */
export const MAX_RESULTS_FILE_BYTES = 32 * 1024 * 1024;

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const CHOOSE_FILE = "Choose the results-<scan ID>.json file the scan command saved.";

export type ParsedResultsFile =
  | { ok: true; file: ShadowScanResultsFile }
  | { ok: false; error: string };

const isObject = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);

/**
 * Reads a results file's text: it must be JSON in the scan command's results
 * format, version 1, with a scan ID and a results object. The results are not
 * checked further here; the server validates them.
 */
export function parseResultsFile(text: string): ParsedResultsFile {
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch {
    return { ok: false, error: `This file is not valid JSON. ${CHOOSE_FILE}` };
  }
  if (!isObject(data) || data.format !== SHADOW_SCAN_RESULTS_FORMAT) {
    return { ok: false, error: `This is not a scan results file. ${CHOOSE_FILE}` };
  }
  if (data.version !== 1) {
    return {
      ok: false,
      error: "This results file has a version Agent Manager cannot read. Update the scan " +
        "command and scan again.",
    };
  }
  if (typeof data.scanId !== "string" || !UUID.test(data.scanId) || !isObject(data.results)) {
    return { ok: false, error: `This results file is incomplete. ${CHOOSE_FILE}` };
  }
  if (new TextEncoder().encode(JSON.stringify(data.results)).length > MAX_RESULTS_BYTES) {
    return {
      ok: false,
      error: "The results in this file are larger than 8 MiB, the most Agent Manager accepts.",
    };
  }
  return { ok: true, file: data as unknown as ShadowScanResultsFile };
}

/** A readable message for a failed upload. */
export function uploadErrorMessage(e: unknown): string {
  if (isRequestTooLargeError(e)) {
    return "The results are larger than this Agent Manager accepts from the console.";
  }
  const status = (e as { status?: number } | null)?.status;
  const message = (e as { message?: string } | null)?.message ?? "";
  switch (status) {
    case 409:
      return /newer/i.test(message)
        ? "A newer scan of this connector has already uploaded its results, so these are out " +
            "of date. Run a new scan."
        : "This scan is no longer waiting for results.";
    case 413:
      return "The results are larger than Agent Manager accepts.";
    case 400:
      return message && !message.startsWith("HTTP error")
        ? `Agent Manager rejected the results: ${message}`
        : "Agent Manager rejected the results. Scan again and upload the new file.";
    case 404:
      return "The scan was not found. It may have been cleared.";
    default:
      return "The results could not be uploaded. Try again.";
  }
}

/** A summary count, or the length of the matching results list when the summary lacks it. */
function num(v: unknown, fallback: unknown[] | undefined): number | undefined {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  return Array.isArray(fallback) ? fallback.length : undefined;
}

function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(typeof reader.result === "string" ? reader.result : "");
    reader.onerror = () => reject(reader.error);
    reader.readAsText(file, "utf-8");
  });
}

function PreviewRow({ label, value }: { label: string; value: string }) {
  return (
    <Stack direction="row" spacing={2}>
      <Typography variant="body2" color="text.secondary" sx={{ width: 160, flexShrink: 0 }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ minWidth: 0, wordBreak: "break-word" }}>
        {value}
      </Typography>
    </Stack>
  );
}

/**
 * Uploads the results file a scan command saved. Opened for one waiting scan,
 * the file must be that scan's; opened without one, the file's scan ID picks
 * the scan, which must be waiting for results. The file is read in the
 * browser and only its results are sent.
 */
export function UploadResultsDialog({
  orgName,
  open,
  scan,
  scans,
  onClose,
}: {
  orgName: string | undefined;
  open: boolean;
  /** The scan the dialog was opened for, if any. */
  scan?: ShadowScanResponse | null;
  /** The organization's scans, newest first. */
  scans: ShadowScanResponse[];
  onClose: () => void;
}) {
  const upload = useUploadShadowScanResults(orgName);
  const inputRef = useRef<HTMLInputElement>(null);
  // The newest file read wins when files are picked one after another.
  const readSeq = useRef(0);
  const [fileName, setFileName] = useState("");
  const [file, setFile] = useState<ShadowScanResultsFile | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);

  const { reset } = upload;
  const clear = () => {
    readSeq.current += 1;
    setFileName("");
    setFile(null);
    setFileError(null);
    setSubmitError(null);
    setDragging(false);
  };
  // Every opening starts empty; reset() drops the last upload from the cache.
  useEffect(() => {
    if (!open) return;
    reset();
    readSeq.current += 1;
    setFileName("");
    setFile(null);
    setFileError(null);
    setSubmitError(null);
    setDragging(false);
  }, [open, reset]);

  const fileScanId = file?.scanId.toLowerCase();
  const listed = fileScanId ? scans.find((s) => s.id.toLowerCase() === fileScanId) : undefined;
  // A scan older than the list is looked up by its ID.
  const lookup = useShadowScan(orgName, open && !scan && file && !listed ? file.scanId : undefined);
  const fixed = scan ? (scans.find((s) => s.id === scan.id) ?? scan) : undefined;
  const target = fixed ?? listed ?? lookup.data;

  let scanError: string | null = null;
  let checking = false;
  if (file) {
    if (fixed && fileScanId !== fixed.id.toLowerCase()) {
      scanError = `This file is for another scan. Choose results-${fixed.id}.json.`;
    } else if (!target) {
      if (lookup.isLoading) checking = true;
      else if (lookup.error && (lookup.error as { status?: number }).status !== 404) {
        scanError = "Couldn't check the scan of this file. Try again.";
      } else {
        scanError = "No scan of this organization has this file's scan ID.";
      }
    } else if (target.status !== "pending") {
      scanError =
        "This scan is no longer waiting for results. Its status is " +
        `${SCAN_STATUS_LABELS[target.status] ?? target.status}.`;
    }
  }

  const read = async (f: File | undefined) => {
    if (!f) return;
    const seq = ++readSeq.current;
    setFile(null);
    setSubmitError(null);
    setFileName(f.name);
    if (f.size > MAX_RESULTS_FILE_BYTES) {
      setFileError("This file is larger than 32 MiB, too large for a results file.");
      return;
    }
    let text: string;
    try {
      text = await readText(f);
    } catch {
      if (seq === readSeq.current) setFileError("The file could not be read. Try again.");
      return;
    }
    if (seq !== readSeq.current) return;
    const parsed = parseResultsFile(text);
    setFileError(parsed.ok ? null : parsed.error);
    setFile(parsed.ok ? parsed.file : null);
  };

  const finish = () => {
    clear();
    reset();
    onClose();
  };
  // Closing is held while the upload runs.
  const close = () => {
    if (!upload.isPending) finish();
  };

  const submit = () => {
    if (!file || !target || scanError || checking) return;
    setSubmitError(null);
    upload.mutate(
      { scanId: target.id, results: file.results },
      {
        onSuccess: () => finish(),
        onError: (e) => setSubmitError(uploadErrorMessage(e)),
      },
    );
  };

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (!upload.isPending) void read(e.dataTransfer.files?.[0]);
  };

  const summary = file?.summary;
  const results = file?.results;
  const scanned = num(summary?.repositoriesScanned, results?.scannedRepositories);
  const failed = num(summary?.repositoriesFailed, results?.failedRepositories);
  const agents = num(summary?.agents, results?.agents);
  const started = target ? formatRelativeTime(target.startedAt ?? target.createdAt) : "";

  return (
    <Dialog open={open} onClose={close} maxWidth="sm" fullWidth>
      <DialogTitle>Upload results</DialogTitle>
      <DialogContent>
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            Upload the results-&lt;scan ID&gt;.json file the scan command saved in the folder you
            ran it from. Only the agents it confirmed are sent.
          </Typography>
          <Box
            role="button"
            tabIndex={0}
            aria-label="Choose a results file"
            onClick={() => !upload.isPending && inputRef.current?.click()}
            onKeyDown={(e) => {
              if (e.key !== "Enter" && e.key !== " ") return;
              e.preventDefault();
              if (!upload.isPending) inputRef.current?.click();
            }}
            onDragOver={(e) => {
              e.preventDefault();
              setDragging(true);
            }}
            onDragLeave={() => setDragging(false)}
            onDrop={onDrop}
            sx={{
              border: 1,
              borderStyle: "dashed",
              borderColor: dragging ? "primary.main" : "divider",
              borderRadius: 1,
              bgcolor: dragging ? "action.hover" : "transparent",
              p: 3,
              textAlign: "center",
              cursor: "pointer",
            }}
          >
            <Stack spacing={1} alignItems="center">
              <FileUp size={24} />
              <Typography variant="body2">
                {fileName || "Drop the results file here, or click to choose it"}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                A .json file with up to 8 MiB of results
              </Typography>
            </Stack>
          </Box>
          <input
            ref={inputRef}
            type="file"
            accept=".json,application/json"
            hidden
            data-testid="results-file-input"
            onChange={(e) => {
              void read(e.target.files?.[0]);
              e.target.value = "";
            }}
          />
          {fileError && <Alert severity="error">{fileError}</Alert>}
          {file && !fileError && scanError && <Alert severity="error">{scanError}</Alert>}
          {file && checking && (
            <Typography variant="caption" color="text.secondary">
              Checking the scan…
            </Typography>
          )}
          {file && target && !scanError && (
            <Stack spacing={1} sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 2 }}>
              <PreviewRow label="Organization" value={summary?.organization || "Not given"} />
              <PreviewRow label="Scan started" value={started} />
              <PreviewRow
                label="Repositories"
                value={`${formatCount(scanned)} scanned, ${formatCount(failed)} failed`}
              />
              <PreviewRow label="Agents" value={formatCount(agents)} />
            </Stack>
          )}
          {submitError && <Alert severity="error">{submitError}</Alert>}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
        <Button
          variant="contained"
          onClick={submit}
          disabled={!file || !target || !!scanError || checking || upload.isPending}
        >
          Upload
        </Button>
        <Button variant="outlined" onClick={close} disabled={upload.isPending}>
          Cancel
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/**
 * Cancel scan: asks to confirm, then the waiting scan becomes Failed.
 * Returns the function that asks.
 */
export function useConfirmCancelScan(orgName: string | undefined) {
  const cancel = useCancelShadowScan(orgName);
  const { addConfirmation } = useConfirmationDialog();
  return (scan: ShadowScanResponse, onDone?: () => void) =>
    addConfirmation({
      title: "Cancel this scan?",
      description:
        "The scan stops waiting for results and shows as Failed. Its results file can no " +
        "longer be uploaded. To scan again, generate a new command.",
      confirmButtonText: "Cancel scan",
      cancelButtonText: "Keep waiting",
      confirmButtonColor: "error",
      onConfirm: () => cancel.mutate(scan.id, { onSuccess: () => onDone?.() }),
    });
}

export default UploadResultsDialog;

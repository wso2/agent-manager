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

import { useRef, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Link,
  ListingTable,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  CheckCircle,
  FileText,
  Upload,
  X,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import { useFeatureManagement } from "@agent-management-platform/feature-management";
import {
  isNetworkError,
  useCheckBudgetImport,
  useImportBudgets,
} from "@agent-management-platform/api-client";
import type {
  BudgetImportOnExisting,
  BudgetImportResult,
} from "@agent-management-platform/types";
import {
  importErrorMessage,
  IMPORT_MAX_BYTES,
  IMPORT_TEMPLATE,
  rowError,
  rowStatus,
  validationResult,
} from "../budgetImport";
import { FORM_SELECT_SX } from "../controlSizes";

interface ImportBudgetsDialogProps {
  open: boolean;
  orgName?: string;
  onClose: () => void;
}

const ON_EXISTING: { value: BudgetImportOnExisting; label: string }[] = [
  { value: "fail", label: "Reject import" },
  { value: "skip", label: "Skip existing budgets" },
  { value: "update", label: "Update existing budgets" },
];

/**
 * Imports budgets from a CSV file: the file is checked as soon as it is chosen (a dry run),
 * each row's outcome is listed, and only a file with no errors can be imported.
 */
export function ImportBudgetsDialog({
  open,
  orgName,
  onClose,
}: ImportBudgetsDialogProps) {
  const canUpdate = useFeatureManagement().hasAccess("cost_write");
  const check = useCheckBudgetImport({ orgName });
  const save = useImportBudgets({ orgName });
  const input = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<{ name: string; csv: string } | null>(null);
  const [fileError, setFileError] = useState("");
  const [onExisting, setOnExisting] = useState<BudgetImportOnExisting>("fail");
  const [result, setResult] = useState<BudgetImportResult | null>(null);

  const reset = () => {
    setFile(null);
    setFileError("");
    setResult(null);
    setOnExisting("fail");
    check.reset();
    save.reset();
  };
  const close = () => {
    if (save.isPending) return;
    reset();
    onClose();
  };

  const runCheck = (csv: string, mode: BudgetImportOnExisting) => {
    setResult(null);
    save.reset();
    check.mutate({ csv, onExisting: mode }, { onSuccess: setResult });
  };

  const choose = async (chosen: File | undefined) => {
    if (!chosen) return;
    setResult(null);
    check.reset();
    if (chosen.size > IMPORT_MAX_BYTES) {
      setFile(null);
      setFileError("The file must be at most 1 MB.");
      return;
    }
    const csv = await chosen.text();
    setFileError("");
    setFile({ name: chosen.name, csv });
    runCheck(csv, onExisting);
  };

  const importNow = () => {
    if (!file) return;
    save.mutate(
      { csv: file.csv, onExisting },
      {
        onSuccess: (r) => {
          if (r.applied) close();
          else setResult(r); // the data changed since the check: show the new errors
        },
      },
    );
  };

  const summary = result?.summary;
  const toChange = summary ? summary.created + summary.updated : 0;
  const failure = check.error ?? save.error;
  const canImport =
    !!file &&
    !!summary &&
    summary.error === 0 &&
    toChange > 0 &&
    !check.isPending;

  return (
    <Dialog open={open} onClose={close} fullWidth maxWidth="sm">
      <DialogTitle sx={{ pr: 6 }}>
        Import budgets
        <IconButton
          aria-label="Close"
          onClick={close}
          sx={{ position: "absolute", right: 12, top: 12 }}
        >
          <X size={18} />
        </IconButton>
      </DialogTitle>
      <DialogContent>
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            Create several budgets at once from a CSV file.
          </Typography>
          <Box>
            <Link
              href={`data:text/csv;charset=utf-8,${encodeURIComponent(IMPORT_TEMPLATE)}`}
              download="budgets-template.csv"
              variant="body2"
            >
              Download CSV template
            </Link>
          </Box>

          <Stack
            direction="row"
            spacing={1.5}
            alignItems="center"
            flexWrap="wrap"
            useFlexGap
          >
            <input
              ref={input}
              type="file"
              accept=".csv,text/csv"
              hidden
              onChange={(e) => {
                void choose(e.target.files?.[0]);
                e.target.value = ""; // selecting the same file again validates it again
              }}
            />
            <Button
              variant="outlined"
              size="small"
              startIcon={<Upload size={16} />}
              onClick={() => input.current?.click()}
              disabled={save.isPending}
            >
              {file ? "Replace file" : "Select CSV file"}
            </Button>
            {file && (
              <Stack
                direction="row"
                spacing={0.75}
                alignItems="center"
                sx={{ minWidth: 0 }}
              >
                <FileText size={16} />
                <Typography variant="body2" sx={{ overflowWrap: "anywhere" }}>
                  {file.name}
                </Typography>
              </Stack>
            )}
          </Stack>
          {fileError && <Alert severity="error">{fileError}</Alert>}

          <Box>
            <Typography
              component="label"
              htmlFor="duplicate-budget-handling"
              variant="body2"
              fontWeight={500}
              sx={{ display: "block", mb: 0.75 }}
            >
              Duplicate budget handling
            </Typography>
            <TextField
              select
              size="small"
              fullWidth
              sx={FORM_SELECT_SX}
              value={onExisting}
              onChange={(e) => {
                const mode = e.target.value as BudgetImportOnExisting;
                setOnExisting(mode);
                if (file) runCheck(file.csv, mode);
              }}
              SelectProps={{ id: "duplicate-budget-handling" }}
              helperText={
                canUpdate ? undefined : "Needs permission to edit budgets."
              }
            >
              {ON_EXISTING.map((o) => (
                <MenuItem
                  key={o.value}
                  value={o.value}
                  disabled={o.value === "update" && !canUpdate}
                >
                  {o.label}
                </MenuItem>
              ))}
            </TextField>
          </Box>

          {check.isPending && (
            <Stack direction="row" spacing={1} alignItems="center">
              <CircularProgress size={16} />
              <Typography variant="body2">Validating…</Typography>
            </Stack>
          )}
          {failure !== null && failure !== undefined && !check.isPending && (
            <Alert severity="error">
              {isNetworkError(failure)
                ? "The server can't be reached. Try again."
                : importErrorMessage(failure)}
            </Alert>
          )}

          {summary && result && !check.isPending && (
            <>
              {(() => {
                const v = validationResult(summary, result.results.length);
                return (
                  <Alert severity={v.severity} sx={{ py: 0.25 }}>
                    <Typography
                      variant="body2"
                      fontWeight={600}
                      component="span"
                    >
                      {v.title}
                    </Typography>{" "}
                    <Typography variant="body2" component="span">
                      {v.text}
                    </Typography>
                  </Alert>
                );
              })()}
              <ListingTable.Container sx={{ maxHeight: 320, overflow: "auto" }}>
                <ListingTable density="compact">
                  <ListingTable.Head>
                    <ListingTable.Row>
                      <ListingTable.Cell>Budget</ListingTable.Cell>
                      <ListingTable.Cell>Status</ListingTable.Cell>
                    </ListingTable.Row>
                  </ListingTable.Head>
                  <ListingTable.Body>
                    {result.results.map((r) => {
                      const status = rowStatus(r);
                      return (
                        <ListingTable.Row
                          key={r.row}
                          sx={{ verticalAlign: "top" }}
                        >
                          <ListingTable.Cell sx={{ overflowWrap: "anywhere" }}>
                            {r.name || `Line ${r.row}`}
                          </ListingTable.Cell>
                          <ListingTable.Cell>
                            <Stack
                              direction="row"
                              spacing={0.75}
                              alignItems="center"
                              sx={{
                                color: status.valid
                                  ? "success.main"
                                  : "error.main",
                              }}
                            >
                              {status.valid ? (
                                <CheckCircle size={14} />
                              ) : (
                                <XCircle size={14} />
                              )}
                              <Typography variant="body2" fontWeight={500}>
                                {status.label}
                              </Typography>
                            </Stack>
                            {(r.errors ?? []).map((e, i) => {
                              const err = rowError(e);
                              return (
                                <Typography
                                  key={i}
                                  variant="caption"
                                  color="text.secondary"
                                  sx={{ display: "block", mt: 0.25 }}
                                >
                                  {err.message}
                                  {err.column && (
                                    <>
                                      {" "}
                                      Field:{" "}
                                      <Box
                                        component="code"
                                        sx={{ fontFamily: "monospace" }}
                                      >
                                        {err.column}
                                      </Box>
                                    </>
                                  )}
                                </Typography>
                              );
                            })}
                          </ListingTable.Cell>
                        </ListingTable.Row>
                      );
                    })}
                  </ListingTable.Body>
                </ListingTable>
              </ListingTable.Container>
            </>
          )}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        <Button variant="outlined" onClick={close} disabled={save.isPending}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={importNow}
          disabled={!canImport || save.isPending}
          startIcon={
            save.isPending ? (
              <CircularProgress size={14} color="inherit" />
            ) : undefined
          }
        >
          Import budgets
        </Button>
      </DialogActions>
    </Dialog>
  );
}

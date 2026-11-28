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

import { useCallback, useId, useMemo, useRef, useState, type DragEvent } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  Collapse,
  IconButton,
  ListingTable,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, ChevronRight, Download, FileUp, X } from "@wso2/oxygen-ui-icons-react";
import { useLocation, useNavigate } from "react-router-dom";
import { returnToState } from "@agent-management-platform/views";
import {
  isRequestTooLargeError,
  useDownloadShadowInventoryTemplate,
} from "@agent-management-platform/api-client";
import {
  formatBytes,
  getMaxRequestBodyBytes,
  type ShadowInventoryField,
  type ShadowInventoryFormat,
  type ShadowInventoryUploadResponse,
} from "@agent-management-platform/types";
import { CodeHint, CUSTOM_PLATFORM, formatCount, unmanagedAgentsPath } from "../shadowUi";
import { scanFindingsPath } from "./ScansTab";

/** The largest inventory file Agent Manager accepts. */
export const MAX_INVENTORY_BYTES = 5 * 1024 * 1024;

/**
 * The largest file the console sends: 5 MB, or less where the deployment caps
 * request bodies (MAX_REQUEST_BODY_BYTES, for a WAF in front of it).
 */
export function inventoryMaxBytes(): number {
  const limit = getMaxRequestBodyBytes();
  return limit > 0 ? Math.min(MAX_INVENTORY_BYTES, limit) : MAX_INVENTORY_BYTES;
}

/** A byte limit for people: "5 MB", "64 KB". */
export function sizeLabel(bytes: number): string {
  const mib = 1024 * 1024;
  if (bytes >= mib) return `${Math.round((bytes / mib) * 10) / 10} MB`;
  return formatBytes(bytes);
}

interface FormatInfo {
  label: string;
  extension: string;
  accept: string;
  shape: string;
}

export const INVENTORY_FORMATS: Record<ShadowInventoryFormat, FormatInfo> = {
  json: {
    label: "JSON",
    extension: ".json",
    accept: ".json,application/json",
    shape: "An object with an agents array, one object per agent.",
  },
  csv: {
    label: "CSV",
    extension: ".csv",
    accept: ".csv,text/csv",
    shape: "The first row is the column names, then one row per agent. Save it as UTF-8.",
  },
  xlsx: {
    label: "Excel",
    extension: ".xlsx",
    accept: ".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    shape: "The first sheet is read: the first row is the column names, then one row per agent.",
  },
};

const FORMAT_ORDER: ShadowInventoryFormat[] = ["json", "csv", "xlsx"];

/** The inventory format of a file name, by its extension. */
export function inventoryFormatOf(fileName: string): ShadowInventoryFormat | undefined {
  const ext = fileName.toLowerCase().match(/\.[^.]+$/)?.[0];
  return FORMAT_ORDER.find((f) => INVENTORY_FORMATS[f].extension === ext);
}

/** Why a file cannot be uploaded, or null when it can. */
export function inventoryFileError(file: File, maxBytes = inventoryMaxBytes()): string | null {
  if (!inventoryFormatOf(file.name)) return "Choose a .json, .csv or .xlsx file.";
  if (file.size === 0) return "This file is empty.";
  if (file.size > maxBytes) {
    return `This file is larger than ${sizeLabel(maxBytes)}, the most Agent Manager accepts.`;
  }
  return null;
}

/** A readable message for a failed inventory upload. */
export function inventoryUploadErrorMessage(e: unknown): string {
  if (isRequestTooLargeError(e)) {
    return "The file is larger than this Agent Manager accepts from the console.";
  }
  const status = (e as { status?: number } | null)?.status;
  const message = (e as { message?: string } | null)?.message ?? "";
  switch (status) {
    case 400:
      return message && !message.startsWith("HTTP error")
        ? message
        : "Agent Manager could not read the file. Check it against the template.";
    case 409:
      return message && !message.startsWith("HTTP error")
        ? message
        : "Another file was uploaded to this source at the same time. Try again.";
    case 413:
      return "The file is larger than 5 MB, the most Agent Manager accepts.";
    case 503:
      return "Agent Manager couldn't record the upload in the audit log, so nothing was " +
        "saved. Try again later.";
    case 403:
      return "You don't have permission to upload agents.";
    case 404:
      return "The source was not found. It may have been deleted.";
    default:
      return "The file could not be uploaded. Try again.";
  }
}

/** labels as "team=support; region=eu" becomes {"team": "support", "region": "eu"}. */
export function labelsObject(example: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const pair of example.split(";")) {
    const i = pair.indexOf("=");
    if (i <= 0) continue;
    out[pair.slice(0, i).trim()] = pair.slice(i + 1).trim();
  }
  return out;
}

/** One example agent as a JSON inventory file. */
export function exampleJson(fields: ShadowInventoryField[]): string {
  const agent: Record<string, unknown> = {};
  for (const f of fields) {
    agent[f.name] = f.name === "labels" ? labelsObject(f.example) : f.example;
  }
  return JSON.stringify({ agents: [agent] }, null, 2);
}

/** A CSV value, quoted when it holds a comma, a quote or a line break. */
export function csvValue(v: string): string {
  return /[",\r\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/** The header line and one example row of a CSV inventory file. */
export function exampleCsv(fields: ShadowInventoryField[]): string {
  return [
    fields.map((f) => csvValue(f.name)).join(","),
    fields.map((f) => csvValue(f.example)).join(","),
  ].join("\n");
}

/** The chosen file, and the format (tab) shown. */
export interface InventoryFileState {
  format: ShadowInventoryFormat;
  setFormat: (f: ShadowInventoryFormat) => void;
  /** The chosen file, whatever tab is shown. */
  file: File | null;
  setFile: (file: File | null) => void;
  reset: () => void;
}

/** The panel's state, held by the page or dialog that uploads the file. */
export function useInventoryFile(): InventoryFileState {
  const [format, setFormat] = useState<ShadowInventoryFormat>("json");
  const [file, setFile] = useState<File | null>(null);
  const reset = useCallback(() => {
    setFile(null);
    setFormat("json");
  }, []);
  return { format, setFormat, file, setFile, reset };
}

/** The fields of the format, one row each. */
function FieldTable({
  fields,
  format,
}: {
  fields: ShadowInventoryField[];
  format: ShadowInventoryFormat;
}) {
  const example = (f: ShadowInventoryField) =>
    format === "json" && f.name === "labels"
      ? JSON.stringify(labelsObject(f.example))
      : f.example;
  return (
    <Box sx={{ overflowX: "auto" }}>
      <ListingTable density="compact" aria-label="Inventory fields">
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell>{format === "json" ? "Field" : "Column"}</ListingTable.Cell>
            <ListingTable.Cell>Required</ListingTable.Cell>
            <ListingTable.Cell>Description</ListingTable.Cell>
            <ListingTable.Cell>Example</ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {fields.map((f) => (
            <ListingTable.Row key={f.name}>
              <ListingTable.Cell>
                <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                  {f.name}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" color={f.required ? "text.primary" : "text.secondary"}>
                  {f.required ? "Yes" : "No"}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography variant="body2" color="text.secondary">
                  {f.description}
                </Typography>
              </ListingTable.Cell>
              <ListingTable.Cell>
                <Typography
                  variant="body2"
                  sx={{ fontFamily: "monospace", wordBreak: "break-word" }}
                >
                  {example(f)}
                </Typography>
              </ListingTable.Cell>
            </ListingTable.Row>
          ))}
        </ListingTable.Body>
      </ListingTable>
    </Box>
  );
}

/** The first two rows of the Excel template, drawn like a sheet. */
function SheetPreview({ fields }: { fields: ShadowInventoryField[] }) {
  const letter = (i: number) =>
    i < 26 ? String.fromCharCode(65 + i) : `A${String.fromCharCode(65 + i - 26)}`;
  const cell = {
    border: 1,
    borderColor: "divider",
    px: 1,
    py: 0.5,
    fontSize: 12,
    whiteSpace: "nowrap",
    textAlign: "left",
  } as const;
  const heading = { ...cell, bgcolor: "action.hover", color: "text.secondary", fontWeight: 400 };
  return (
    <Box sx={{ overflowX: "auto", border: 1, borderColor: "divider", borderRadius: 1 }}>
      <Box
        component="table"
        aria-label="Example Excel sheet"
        sx={{ borderCollapse: "collapse", minWidth: "100%" }}
      >
        <thead>
          <tr>
            <Box component="th" sx={heading} aria-hidden />
            {fields.map((f, i) => (
              <Box component="th" key={f.name} sx={{ ...heading, textAlign: "center" }}>
                {letter(i)}
              </Box>
            ))}
          </tr>
        </thead>
        <tbody>
          <tr>
            <Box component="th" sx={heading}>
              1
            </Box>
            {fields.map((f) => (
              <Box component="td" key={f.name} sx={{ ...cell, fontWeight: 600 }}>
                {f.name}
              </Box>
            ))}
          </tr>
          <tr>
            <Box component="th" sx={heading}>
              2
            </Box>
            {fields.map((f) => (
              <Box component="td" key={f.name} sx={cell}>
                {f.example}
              </Box>
            ))}
          </tr>
        </tbody>
      </Box>
    </Box>
  );
}

/** What an inventory upload gave: agents saved, rows left out or saved with a warning. */
export function InventoryUploadResult({
  orgName,
  result,
  format,
}: {
  orgName: string | undefined;
  result: ShadowInventoryUploadResponse;
  /** The uploaded file's format: rows for CSV and Excel, agents for JSON. */
  format: ShadowInventoryFormat;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const [showDetails, setShowDetails] = useState(false);
  const detailsId = useId();
  const unit = format === "json" ? "agent" : "row";
  const Unit = format === "json" ? "Agent" : "Row";
  const plural = (n: number, one: string) => `${formatCount(n)} ${n === 1 ? one : `${one}s`}`;
  const leftOut = result.problems.filter((p) => p.leftOut);
  const warnings = result.problems.filter((p) => !p.leftOut);
  const total = result.problemCount ?? result.problems.length;
  const truncated = total > result.problems.length;
  // Each left-out row has one problem; a saved row can have several warnings.
  const warningTotal = Math.max(warnings.length, total - result.leftOut);
  const warnedRows = new Set(warnings.map((p) => p.row)).size;
  const warningLine =
    warningTotal === 0
      ? ""
      : truncated
        ? `${plural(warningTotal, "warning")} on saved ${unit}s.`
        : `${plural(warnedRows, unit)} saved with a warning.`;
  const list = (title: string, items: typeof leftOut) =>
    items.length > 0 && (
      <Box sx={{ mt: 0.5 }}>
        <Typography variant="body2" fontWeight={500}>
          {title}
        </Typography>
        <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
          {items.map((p, i) => (
            <Typography component="li" variant="body2" key={`${p.row}:${i}`}>
              {Unit} {p.row}: {p.message}
            </Typography>
          ))}
        </Box>
      </Box>
    );
  return (
    <Alert
      severity={result.leftOut > 0 || warningTotal > 0 ? "warning" : "success"}
      action={
        <Stack direction={{ xs: "column", sm: "row" }} spacing={0.5}>
          <Button
            color="inherit"
            size="small"
            onClick={() => navigate(unmanagedAgentsPath(orgName ?? "", CUSTOM_PLATFORM))}
          >
            View agents
          </Button>
          <Button
            color="inherit"
            size="small"
            onClick={() =>
              navigate(scanFindingsPath(orgName ?? "", result.scan.id), {
                state: returnToState(location),
              })
            }
          >
            View scan
          </Button>
        </Stack>
      }
    >
      <Typography variant="body2" fontWeight={500}>
        {plural(result.agentsSaved, "agent")} saved
        {result.leftOut > 0 ? `, ${plural(result.leftOut, unit)} left out` : ""}.
        {warningLine ? ` ${warningLine}` : ""}
      </Typography>
      {result.ignoredColumns.length > 0 && (
        <Typography variant="body2" sx={{ mt: 0.5 }}>
          Ignored {result.ignoredColumns.length === 1 ? "column" : "columns"} (not stored):{" "}
          {result.ignoredColumns.join(", ")}
        </Typography>
      )}
      {result.problems.length > 0 && (
        <Box sx={{ mt: 0.5 }}>
          <Button
            color="inherit"
            size="small"
            variant="text"
            startIcon={showDetails ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
            aria-expanded={showDetails}
            aria-controls={detailsId}
            onClick={() => setShowDetails(!showDetails)}
            sx={{ ml: -1 }}
          >
            {showDetails ? "Hide details" : "Show details"}
          </Button>
          <Collapse in={showDetails} id={detailsId}>
            {list("Left out", leftOut)}
            {list("Saved with a warning", warnings)}
            {truncated && (
              <Typography variant="caption" component="p" sx={{ mt: 0.5 }}>
                Showing the first {formatCount(result.problems.length)} of {formatCount(total)}.
              </Typography>
            )}
          </Collapse>
        </Box>
      )}
    </Alert>
  );
}

/**
 * The inventory file format, in JSON, CSV and Excel tabs: a template to
 * download, where to choose the file, then the fields and an example. The
 * file area comes first so it (and, after an upload, the result that takes
 * its place) stays in view above the format.
 */
export function InventoryFilePanel({
  orgName,
  fields,
  state,
  result,
  error,
  disabled = false,
}: {
  orgName: string | undefined;
  fields: ShadowInventoryField[];
  state: InventoryFileState;
  result?: ShadowInventoryUploadResponse | null;
  /** Why the last upload failed. */
  error?: string | null;
  /** While saving or uploading: the file cannot be changed. */
  disabled?: boolean;
}) {
  const { format, setFormat, file, setFile } = state;
  const info = INVENTORY_FORMATS[format];
  const fileFormat = file ? inventoryFormatOf(file.name) : undefined;
  const maxBytes = inventoryMaxBytes();
  const template = useDownloadShadowInventoryTemplate(orgName);
  const inputRef = useRef<HTMLInputElement>(null);
  const [fileError, setFileError] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);
  const baseId = useId();
  const preview = useMemo(
    () => (format === "json" ? exampleJson(fields) : format === "csv" ? exampleCsv(fields) : ""),
    [fields, format],
  );

  const choose = (f: File | undefined) => {
    if (!f || disabled) return;
    const problem = inventoryFileError(f, maxBytes);
    setFileError(problem);
    if (problem) return;
    // A file of another format opens that format's tab.
    setFormat(inventoryFormatOf(f.name) ?? format);
    setFile(f);
  };
  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setDragging(false);
    choose(e.dataTransfer.files?.[0]);
  };
  const open = () => {
    if (!disabled) inputRef.current?.click();
  };

  return (
    <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1 }}>
      <Box sx={{ px: 2, pt: 2 }}>
        <Typography variant="subtitle2">Inventory file</Typography>
        <Typography variant="body2" color="text.secondary">
          Each upload replaces this source&apos;s agents: agents not in the newest file are
          marked as gone (no longer listed), the same as after a scan. Columns not in the format
          are ignored and never stored.
        </Typography>
      </Box>
      <Tabs
        value={format}
        onChange={(_e, v: ShadowInventoryFormat) => {
          setFileError(null);
          setFormat(v);
        }}
        aria-label="Inventory file format"
        sx={{ px: 2, borderBottom: 1, borderColor: "divider" }}
      >
        {FORMAT_ORDER.map((f) => (
          <Tab
            key={f}
            value={f}
            label={INVENTORY_FORMATS[f].label}
            id={`${baseId}-tab-${f}`}
            aria-controls={`${baseId}-panel`}
          />
        ))}
      </Tabs>
      <Stack
        spacing={2}
        sx={{ p: 2 }}
        role="tabpanel"
        id={`${baseId}-panel`}
        aria-labelledby={`${baseId}-tab-${format}`}
      >
        <Stack
          direction={{ xs: "column", sm: "row" }}
          spacing={1.5}
          alignItems={{ sm: "center" }}
          justifyContent="space-between"
        >
          <Typography variant="body2">{info.shape}</Typography>
          <Button
            variant="outlined"
            size="small"
            startIcon={<Download size={16} />}
            disabled={template.isPending}
            onClick={() => template.mutate(format)}
            sx={{ flexShrink: 0 }}
          >
            Download template
          </Button>
        </Stack>
        {result ? (
          <InventoryUploadResult
            orgName={orgName}
            result={result}
            format={fileFormat ?? format}
          />
        ) : (
          <Box>
            {file ? (
              <Stack
                direction="row"
                spacing={1.5}
                alignItems="center"
                sx={{ border: 1, borderColor: "divider", borderRadius: 1, px: 2, py: 1 }}
              >
                <Box sx={{ display: "flex", color: "text.secondary" }} aria-hidden>
                  <FileUp size={20} />
                </Box>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="body2" noWrap>
                      {file.name}
                    </Typography>
                    {fileFormat && (
                      <Chip
                        size="small"
                        variant="outlined"
                        label={INVENTORY_FORMATS[fileFormat].label}
                      />
                    )}
                  </Stack>
                  <Typography variant="caption" color="text.secondary">
                    {formatBytes(file.size)}
                  </Typography>
                </Box>
                <Tooltip title="Remove file">
                  <span>
                    <IconButton
                      size="small"
                      aria-label={`Remove ${file.name}`}
                      disabled={disabled}
                      onClick={() => setFile(null)}
                    >
                      <X size={16} />
                    </IconButton>
                  </span>
                </Tooltip>
              </Stack>
            ) : (
              <Box
                role="button"
                tabIndex={disabled ? -1 : 0}
                aria-label={`Choose a ${info.extension} file`}
                aria-disabled={disabled}
                onClick={open}
                onKeyDown={(e) => {
                  if (e.key !== "Enter" && e.key !== " ") return;
                  e.preventDefault();
                  open();
                }}
                onDragOver={(e) => {
                  e.preventDefault();
                  if (!disabled) setDragging(true);
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
                  cursor: disabled ? "default" : "pointer",
                }}
              >
                <Stack spacing={1} alignItems="center">
                  <FileUp size={24} />
                  <Typography variant="body2">
                    Drop your {info.extension} file here, or click to choose it
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    Up to {sizeLabel(maxBytes)} and 5,000 agents
                  </Typography>
                </Stack>
              </Box>
            )}
            <input
              ref={inputRef}
              type="file"
              accept={info.accept}
              hidden
              data-testid="inventory-file-input"
              onChange={(e) => {
                choose(e.target.files?.[0]);
                e.target.value = "";
              }}
            />
          </Box>
        )}
        {fileError && !result && <Alert severity="error">{fileError}</Alert>}
        {error && !result && <Alert severity="error">{error}</Alert>}
        <FieldTable fields={fields} format={format} />
        <Box>
          <Typography variant="body2" fontWeight={500}>
            Example
          </Typography>
          {format === "xlsx" ? (
            <Box sx={{ mt: 1 }}>
              <SheetPreview fields={fields} />
            </Box>
          ) : (
            <CodeHint code={preview} label={`Example ${info.label} file`} />
          )}
        </Box>
      </Stack>
    </Box>
  );
}

export default InventoryFilePanel;

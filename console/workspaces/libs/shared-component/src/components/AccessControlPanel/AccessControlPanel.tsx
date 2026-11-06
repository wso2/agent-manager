/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useMemo, useRef, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Form,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  FileCode,
  HelpCircle,
  Inbox,
  Upload,
} from "@wso2/oxygen-ui-icons-react";
import ExpandableResourceRow from "./ExpandableResourceRow";

export type AccessControlMode = "allow" | "deny";

export type AccessControlItem = {
  /** Stable, unique key identifying this item. */
  key: string;
  /** Short label shown as the leading chip (HTTP method, or capability kind). */
  method: string;
  /** Primary text (resource path, tool name, resource URI, prompt name). */
  path: string;
  /** Secondary text shown under the primary line. */
  summary?: string;
  /** Optional OpenAPI fragment for the swagger viewer in the expanded row. */
  operationSpec?: Record<string, unknown> | null;
};

export type AccessControlStatus = {
  message: string;
  severity: "success" | "error";
};

export interface AccessControlPanelProps {
  /** All items available for inclusion in the allowed/exception lists. */
  items: AccessControlItem[];
  /** Current mode. Parent owns this state. */
  mode: AccessControlMode;
  /** Called when the user confirms a mode change. */
  onModeChange: (mode: AccessControlMode) => void;
  /** Keys of items that are exceptions to the current mode. */
  exceptionKeys: string[];
  /** Called when the exception set changes. */
  onExceptionKeysChange: (keys: string[]) => void;
  /** Whether the panel is in an initial loading state. */
  isLoading?: boolean;
  /** Whether a save call is in flight. */
  isSaving?: boolean;
  /** Whether unsaved changes exist; controls Save/Discard enablement. */
  isDirty: boolean;
  /** Triggered when the user clicks Save. */
  onSave: () => void;
  /** Triggered when the user clicks Discard. */
  onDiscard: () => void;
  /** Optional status alert. */
  status?: AccessControlStatus | null;
  /** Called when the user dismisses the status alert. */
  onClearStatus?: () => void;
  /** Show the "Import from specification" button. */
  showImportButton?: boolean;
  /** Shows the lists without move, mode, import or save controls. */
  readOnly?: boolean;
  /** Called when the user picks a spec file. Parent handles parsing. */
  onImportSpec?: (file: File) => void | Promise<void>;
  /** Empty-state copy for the available column. */
  availableEmptyTitle?: string;
  availableEmptyDescription?: string;
  /** Empty-state copy for the exceptions column. */
  exceptionsEmptyTitle?: string;
  exceptionsEmptyDescription?: string;
}

const ALLOW_TOOLTIP =
  "Allow all exposes every resource. Exceptions are hidden.";
const DENY_TOOLTIP = "Deny all hides every resource. Exceptions are exposed.";

export const AccessControlPanel: React.FC<AccessControlPanelProps> = ({
  items,
  mode,
  onModeChange,
  exceptionKeys,
  onExceptionKeysChange,
  isLoading = false,
  isSaving = false,
  isDirty,
  onSave,
  onDiscard,
  status,
  onClearStatus,
  showImportButton = false,
  readOnly = false,
  onImportSpec,
  availableEmptyTitle = "No available resources",
  availableEmptyDescription = "Resources will appear here once they are loaded.",
  exceptionsEmptyTitle = "No selected resources",
  exceptionsEmptyDescription = "Use the arrow buttons to move resources between the lists.",
}) => {
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [pendingMode, setPendingMode] = useState<AccessControlMode | null>(
    null,
  );
  const [modeConfirmOpen, setModeConfirmOpen] = useState(false);
  const [availableSearch, setAvailableSearch] = useState("");
  const [exceptionSearch, setExceptionSearch] = useState("");
  const [selectedAvailableKeys, setSelectedAvailableKeys] = useState<string[]>(
    [],
  );
  const [selectedExceptionKeys, setSelectedExceptionKeys] = useState<string[]>(
    [],
  );
  const [openKey, setOpenKey] = useState<string | null>(null);

  const exceptionKeySet = useMemo(
    () => new Set(exceptionKeys),
    [exceptionKeys],
  );

  const itemsByKey = useMemo(() => {
    const map = new Map<string, AccessControlItem>();
    items.forEach((item) => map.set(item.key, item));
    return map;
  }, [items]);

  const availableItems = useMemo(
    () => items.filter((item) => !exceptionKeySet.has(item.key)),
    [items, exceptionKeySet],
  );

  const exceptionItems = useMemo(() => {
    return exceptionKeys
      .map((key) => itemsByKey.get(key))
      .filter((item): item is AccessControlItem => Boolean(item));
  }, [exceptionKeys, itemsByKey]);

  const filteredAvailableItems = useMemo(() => {
    const query = availableSearch.trim().toLowerCase();
    if (!query) return availableItems;
    return availableItems.filter((item) => {
      const haystack = `${item.method} ${item.path} ${item.summary ?? ""}`
        .toLowerCase();
      return haystack.includes(query);
    });
  }, [availableItems, availableSearch]);

  const filteredExceptionItems = useMemo(() => {
    const query = exceptionSearch.trim().toLowerCase();
    if (!query) return exceptionItems;
    return exceptionItems.filter((item) => {
      const haystack = `${item.method} ${item.path} ${item.summary ?? ""}`
        .toLowerCase();
      return haystack.includes(query);
    });
  }, [exceptionItems, exceptionSearch]);

  const selectedAvailableKeySet = useMemo(
    () => new Set(selectedAvailableKeys),
    [selectedAvailableKeys],
  );
  const selectedExceptionKeySet = useMemo(
    () => new Set(selectedExceptionKeys),
    [selectedExceptionKeys],
  );

  const handleModeChange = (
    _event: React.MouseEvent<HTMLElement>,
    newMode: AccessControlMode | null,
  ) => {
    if (!newMode || newMode === mode) return;
    setPendingMode(newMode);
    setModeConfirmOpen(true);
  };

  const handleCancelModeChange = () => {
    setModeConfirmOpen(false);
    setPendingMode(null);
  };

  const handleApplyModeChange = () => {
    if (!pendingMode || pendingMode === mode) {
      handleCancelModeChange();
      return;
    }
    onModeChange(pendingMode);
    setModeConfirmOpen(false);
    setPendingMode(null);
  };

  const toggleAvailableSelection = (item: AccessControlItem) => {
    setSelectedAvailableKeys((prev) =>
      prev.includes(item.key)
        ? prev.filter((k) => k !== item.key)
        : [...prev, item.key],
    );
  };

  const toggleExceptionSelection = (item: AccessControlItem) => {
    setSelectedExceptionKeys((prev) =>
      prev.includes(item.key)
        ? prev.filter((k) => k !== item.key)
        : [...prev, item.key],
    );
  };

  const moveSelectedToExceptions = () => {
    if (!selectedAvailableKeys.length) return;
    const toAdd = availableItems
      .filter((item) => selectedAvailableKeySet.has(item.key))
      .map((item) => item.key);
    if (!toAdd.length) return;
    onExceptionKeysChange([...exceptionKeys, ...toAdd]);
    setSelectedAvailableKeys([]);
  };

  const moveSelectedToAvailable = () => {
    if (!selectedExceptionKeys.length) return;
    onExceptionKeysChange(
      exceptionKeys.filter((key) => !selectedExceptionKeySet.has(key)),
    );
    setSelectedExceptionKeys([]);
  };

  const moveAllToExceptions = () => {
    onExceptionKeysChange(items.map((item) => item.key));
    setSelectedAvailableKeys([]);
  };

  const moveAllToAvailable = () => {
    onExceptionKeysChange([]);
    setSelectedExceptionKeys([]);
  };

  const handleUploadClick = () => fileInputRef.current?.click();

  const handleFileChange = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const file = event.target.files?.[0];
    if (file && onImportSpec) {
      await onImportSpec(file);
    }
    event.target.value = "";
  };

  type EmptyStateConfig = {
    illustration: React.ReactNode;
    title: string;
    description: string;
  };

  const renderItemList = (
    listItems: AccessControlItem[],
    selectedKeySet: Set<string>,
    onToggle: (item: AccessControlItem) => void,
    emptyState: EmptyStateConfig,
  ) => (
    <Box sx={{ flex: 1, minHeight: 0, overflowY: "auto", pr: 0.5 }}>
      <Stack spacing={1.25} minHeight="100%">
        {listItems.map((item) => {
          const isOpen = openKey === item.key;
          return (
            <ExpandableResourceRow
              key={item.key}
              resource={{
                method: item.method,
                path: item.path,
                summary: item.summary,
              }}
              isOpen={isOpen}
              operationSpec={item.operationSpec}
              selected={selectedKeySet.has(item.key)}
              onRowClick={() => onToggle(item)}
              onToggleOpen={() => setOpenKey(isOpen ? null : item.key)}
            />
          );
        })}
        {!listItems.length && (
          <ListingTable.Container sx={{ flex: 1, minHeight: 0 }}>
            <ListingTable.EmptyState
              illustration={emptyState.illustration}
              title={emptyState.title}
              description={emptyState.description}
            />
          </ListingTable.Container>
        )}
      </Stack>
    </Box>
  );

  if (isLoading) {
    return (
      <Stack spacing={2}>
        <Skeleton variant="rounded" height={48} />
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: "1fr auto 1fr",
            gap: 2,
          }}
        >
          <Skeleton variant="rounded" height={400} />
          <Skeleton variant="rounded" height={200} />
          <Skeleton variant="rounded" height={400} />
        </Box>
      </Stack>
    );
  }

  const isError = !!status && (status.severity === "error" || !isDirty);

  return (
    <Box height={isError ? "calc(100vh - 370px)" : "calc(100vh - 420px)"}>
      <Stack spacing={2}>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            flexWrap: "wrap",
            gap: 2,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Typography variant="body1">Mode</Typography>
            <ToggleButtonGroup
              size="small"
              value={mode}
              exclusive
              disabled={readOnly}
              onChange={handleModeChange}
            >
              <ToggleButton
                color="primary"
                value="allow"
                sx={{ textTransform: "none" }}
              >
                Allow all
                <Tooltip arrow title={ALLOW_TOOLTIP}>
                  <IconButton size="small">
                    <HelpCircle size={16} />
                  </IconButton>
                </Tooltip>
              </ToggleButton>
              <ToggleButton
                color="primary"
                value="deny"
                sx={{ textTransform: "none" }}
              >
                Deny all
                <Tooltip arrow title={DENY_TOOLTIP}>
                  <IconButton size="small">
                    <HelpCircle size={16} />
                  </IconButton>
                </Tooltip>
              </ToggleButton>
            </ToggleButtonGroup>
          </Box>

          {showImportButton && !readOnly && (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Button
                variant="outlined"
                size="small"
                startIcon={<Upload size={16} />}
                onClick={handleUploadClick}
                disabled={isSaving}
              >
                Import from specification
              </Button>
              <input
                ref={fileInputRef}
                type="file"
                hidden
                accept=".json,.yaml,.yml"
                onChange={handleFileChange}
              />
            </Box>
          )}
        </Box>

        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", md: readOnly ? "1fr 1fr" : "1fr auto 1fr" },
            gap: 2,
          }}
        >
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              overflow: "hidden",
              overflowY: "scroll",
              flex: 1,
              minHeight: 0,
            }}
          >
            <Form.Section>
              <Form.Header>
                {mode === "allow" ? "Allowed Resources" : "Denied Resources"}
              </Form.Header>
              <Form.Stack spacing={1.5} height="calc(100vh - 620px)">
                <TextField
                  size="small"
                  placeholder="Search resources"
                  value={availableSearch}
                  fullWidth
                  onChange={(e) => setAvailableSearch(e.target.value)}
                />
                {renderItemList(
                  filteredAvailableItems,
                  selectedAvailableKeySet,
                  toggleAvailableSelection,
                  {
                    illustration: <FileCode size={64} />,
                    title: availableEmptyTitle,
                    description: availableEmptyDescription,
                  },
                )}
              </Form.Stack>
            </Form.Section>
          </Box>

          {!readOnly && (
          <Box
            sx={{
              display: "flex",
              alignItems: "flex-start",
              justifyContent: "center",
              marginTop: 15,
            }}
          >
            <Stack spacing={1}>
              <Tooltip title="Move all to exceptions" arrow>
                <span>
                  <IconButton
                    size="small"
                    onClick={moveAllToExceptions}
                    disabled={!availableItems.length || isSaving}
                    sx={{ border: "1px solid", borderColor: "divider" }}
                  >
                    <ChevronsRight size={18} />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Move selected to exceptions" arrow>
                <span>
                  <IconButton
                    size="small"
                    onClick={moveSelectedToExceptions}
                    disabled={!selectedAvailableKeys.length || isSaving}
                    sx={{ border: "1px solid", borderColor: "divider" }}
                  >
                    <ChevronRight size={18} />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Move selected to available" arrow>
                <span>
                  <IconButton
                    size="small"
                    onClick={moveSelectedToAvailable}
                    disabled={!selectedExceptionKeys.length || isSaving}
                    sx={{ border: "1px solid", borderColor: "divider" }}
                  >
                    <ChevronLeft size={18} />
                  </IconButton>
                </span>
              </Tooltip>
              <Tooltip title="Move all to available" arrow>
                <span>
                  <IconButton
                    size="small"
                    onClick={moveAllToAvailable}
                    disabled={!exceptionItems.length || isSaving}
                    sx={{ border: "1px solid", borderColor: "divider" }}
                  >
                    <ChevronsLeft size={18} />
                  </IconButton>
                </span>
              </Tooltip>
            </Stack>
          </Box>
          )}

          <Form.Section>
            <Form.Header>
              {mode === "allow" ? "Denied Resources" : "Allowed Resources"}
            </Form.Header>
            <Form.Stack spacing={1.5} height="calc(100vh - 620px)">
              <TextField
                size="small"
                placeholder="Search resources"
                value={exceptionSearch}
                fullWidth
                onChange={(e) => setExceptionSearch(e.target.value)}
              />
              {renderItemList(
                filteredExceptionItems,
                selectedExceptionKeySet,
                toggleExceptionSelection,
                {
                  illustration: <Inbox size={64} />,
                  title: exceptionsEmptyTitle,
                  description: exceptionsEmptyDescription,
                },
              )}
            </Form.Stack>
          </Form.Section>
        </Box>

        <Stack spacing={1.5} width="100%">
          <Collapse in={isError} timeout={300}>
            {status && (
              <Alert
                severity={status.severity}
                onClose={onClearStatus}
                sx={{ width: "100%" }}
              >
                {status.message}
              </Alert>
            )}
          </Collapse>
          {!readOnly && (
          <Stack direction="row" spacing={1.5} justifyContent="flex-end">
            <Button
              variant="outlined"
              onClick={onDiscard}
              disabled={!isDirty || isSaving}
            >
              Discard
            </Button>
            <Button
              variant="contained"
              onClick={onSave}
              disabled={!isDirty || isSaving}
            >
              {isSaving ? "Saving..." : "Save"}
            </Button>
          </Stack>
          )}
        </Stack>
      </Stack>

      <Dialog
        open={modeConfirmOpen}
        onClose={handleCancelModeChange}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Confirm Resource Mode Change</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Change resource mode from {mode === "allow" ? "Allow all" : "Deny all"} to{" "}
            {pendingMode === "allow" ? "Allow all" : "Deny all"}?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCancelModeChange}>Cancel</Button>
          <Button
            variant="contained"
            onClick={handleApplyModeChange}
            disabled={!pendingMode || isSaving}
          >
            Apply
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

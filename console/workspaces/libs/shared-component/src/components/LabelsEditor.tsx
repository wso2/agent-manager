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

import { useState } from "react";
import { Box, Button, Chip, IconButton, Typography } from "@wso2/oxygen-ui";
import { Plus as Add, X as RemoveIcon } from "@wso2/oxygen-ui-icons-react";
import { TextInput } from "@agent-management-platform/views";
import { INPUT_LIMITS } from "@agent-management-platform/types";

/** Mirrors the backend label rules (utils/labels.go in agent-manager-service). */
export const MAX_LABELS_PER_RESOURCE = 10;
const LABEL_KEY_REGEX = /^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$/;
const LABEL_VALUE_REGEX = /^([a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?)?$/;
const MAX_LABEL_LENGTH = 63;

export function validateLabelKey(key: string): string | undefined {
  if (!key) return "Key is required";
  if (key.length > MAX_LABEL_LENGTH)
    return `Key must be at most ${MAX_LABEL_LENGTH} characters`;
  if (!LABEL_KEY_REGEX.test(key))
    return "Use letters, digits, '.', '_' or '-', starting and ending with a letter or digit";
  return undefined;
}

export function validateLabelValue(value: string): string | undefined {
  if (value.length > MAX_LABEL_LENGTH)
    return `Value must be at most ${MAX_LABEL_LENGTH} characters`;
  if (!LABEL_VALUE_REGEX.test(value))
    return "Use letters, digits, '.', '_' or '-', starting and ending with a letter or digit";
  return undefined;
}

interface LabelsEditorProps {
  /** Current labels as a key/value map. */
  value: Record<string, string>;
  onChange: (labels: Record<string, string>) => void;
  disabled?: boolean;
  /** When true, the section title is hidden */
  hideTitle?: boolean;
  title?: string;
  description?: string;
}

/**
 * Key/value label editor: existing labels render as removable chips, new
 * labels are added through a small inline form with the same validation rules
 * the backend enforces.
 */
export const LabelsEditor = ({
  value,
  onChange,
  disabled = false,
  hideTitle = false,
  title = "Labels (Optional)",
  description = "Attach key/value labels to organize and filter resources.",
}: LabelsEditorProps) => {
  const [isAddFormOpen, setIsAddFormOpen] = useState(false);
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  const entries = Object.entries(value);
  const keyError = newKey ? validateLabelKey(newKey) : undefined;
  const valueError = validateLabelValue(newValue);
  const duplicateError =
    newKey && Object.prototype.hasOwnProperty.call(value, newKey)
      ? "A label with this key already exists"
      : undefined;
  const atCapacity = entries.length >= MAX_LABELS_PER_RESOURCE;
  const isAddDisabled =
    !newKey || !!keyError || !!valueError || !!duplicateError;

  const handleAdd = () => {
    if (isAddDisabled) return;
    onChange({ ...value, [newKey]: newValue });
    setNewKey("");
    setNewValue("");
    setIsAddFormOpen(false);
  };

  const handleRemove = (key: string) => {
    const next = { ...value };
    delete next[key];
    onChange(next);
  };

  const handleCancel = () => {
    setNewKey("");
    setNewValue("");
    setIsAddFormOpen(false);
  };

  return (
    <Box display="flex" flexDirection="column" gap={1.5} width="100%">
      {!hideTitle && <Typography variant="h6">{title}</Typography>}
      <Typography variant="body2">{description}</Typography>

      {entries.length > 0 && (
        <Box display="flex" flexWrap="wrap" gap={1}>
          {entries.map(([key, val]) => (
            <Chip
              key={key}
              label={val ? `${key}: ${val}` : key}
              size="small"
              variant="outlined"
              onDelete={disabled ? undefined : () => handleRemove(key)}
              deleteIcon={<RemoveIcon size={14} />}
            />
          ))}
        </Box>
      )}

      {isAddFormOpen && (
        <Box display="flex" gap={1} alignItems="flex-start">
          <TextInput
            maxLength={INPUT_LIMITS.KEY}
            label="Key"
            size="small"
            value={newKey}
            onChange={(e) => setNewKey(e.target.value)}
            placeholder="e.g. env"
            error={!!keyError || !!duplicateError}
            helperText={keyError ?? duplicateError}
            disabled={disabled}
          />
          <TextInput
            maxLength={INPUT_LIMITS.VALUE}
            label="Value"
            size="small"
            value={newValue}
            onChange={(e) => setNewValue(e.target.value)}
            placeholder="e.g. production"
            error={!!valueError}
            helperText={valueError}
            disabled={disabled}
          />
          <Box display="flex" gap={0.5} mt={2.5}>
            <Button
              variant="contained"
              size="small"
              onClick={handleAdd}
              disabled={disabled || isAddDisabled}
            >
              Add
            </Button>
            <IconButton size="small" onClick={handleCancel} title="Cancel">
              <RemoveIcon size={18} />
            </IconButton>
          </Box>
        </Box>
      )}

      {!isAddFormOpen && !disabled && (
        <Box display="flex" justifyContent="flex-start">
          <Button
            startIcon={<Add fontSize="small" />}
            variant="outlined"
            color="primary"
            size="small"
            onClick={() => setIsAddFormOpen(true)}
            disabled={atCapacity}
            title={
              atCapacity
                ? `At most ${MAX_LABELS_PER_RESOURCE} labels per resource`
                : undefined
            }
          >
            Add Label
          </Button>
        </Box>
      )}
    </Box>
  );
};

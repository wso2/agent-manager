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

import React, { useMemo, useState } from "react";
import {
  Button,
  Chip,
  Form,
  FormControl,
  FormHelperText,
  IconButton,
  MenuItem,
  Select,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { FieldRendererProps } from "./types";

function keyToLabel(name: string): string {
  return name
    .replace(/([A-Z])/g, " $1")
    .replace(/[_-]+/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/^./, (s) => s.toUpperCase());
}

function fieldLabel(node: FieldRendererProps["node"]): string {
  const base = keyToLabel(node.name);
  return node.isRequired ? `* ${base}` : base;
}

// ---------------------------------------------------------------------------
// SimpleTagInput
// ---------------------------------------------------------------------------

type SimpleTagInputProps = {
  value: string[];
  onChange: (value: string[]) => void;
  placeholder?: string;
  disabled?: boolean;
  error?: boolean;
  helperText?: string;
};

const SimpleTagInput: React.FC<SimpleTagInputProps> = ({
  value,
  onChange,
  placeholder,
  disabled,
  error,
  helperText,
}) => {
  const [inputValue, setInputValue] = useState("");

  const normalizedValues = useMemo(
    () => value.map((item) => item.trim()).filter(Boolean),
    [value],
  );

  const addTag = (raw: string) => {
    const trimmed = raw.trim();
    if (!trimmed || normalizedValues.includes(trimmed)) return;
    onChange([...normalizedValues, trimmed]);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      addTag(inputValue);
      setInputValue("");
      return;
    }
    if (e.key === "Backspace" && inputValue === "" && value.length > 0) {
      onChange(normalizedValues.slice(0, -1));
    }
  };

  return (
    <Stack spacing={0.75} pt={1}>
      {normalizedValues.length > 0 && (
        <Stack direction="row" flexWrap="wrap" spacing={0.5} useFlexGap>
          {normalizedValues.map((tag) => (
            <Chip
              key={tag}
              label={tag}
              size="small"
              onDelete={
                disabled
                  ? undefined
                  : () => onChange(normalizedValues.filter((t) => t !== tag))
              }
            />
          ))}
        </Stack>
      )}
      <TextField
        value={inputValue}
        onChange={(e) => setInputValue(e.target.value)}
        onKeyDown={handleKeyDown}
        disabled={disabled}
        placeholder={placeholder}
        size="small"
        error={!!error}
      />
      {helperText && (
        <FormHelperText error={!!error}>{helperText}</FormHelperText>
      )}
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// Field renderers
// ---------------------------------------------------------------------------

export const StringFieldRenderer: React.FC<FieldRendererProps> = ({
  node,
  value,
  onChange,
  error,
  disabled,
}) => {
  const label = fieldLabel(node);
  const helperText = error || node.schema.description;

  if (node.schema.enum && node.schema.enum.length > 0) {
    return (
      <Stack pb={2}>
        <Form.ElementWrapper label={label} name={node.path}>
          <FormControl
            fullWidth
            size="small"
            error={!!error}
            disabled={disabled}
          >
            <Select
              value={(value as string) || ""}
              onChange={(e) => onChange(node.path, e.target.value as string)}
              displayEmpty
              variant="outlined"
            >
              <MenuItem value="">
                <em>Select...</em>
              </MenuItem>
              {node.schema.enum!.map((opt) => (
                <MenuItem key={opt} value={opt}>
                  {opt}
                </MenuItem>
              ))}
            </Select>
            {helperText && (
              <FormHelperText error={!!error}>{helperText}</FormHelperText>
            )}
          </FormControl>
        </Form.ElementWrapper>
      </Stack>
    );
  }

  return (
    <Stack spacing={0.75} pb={2}>
      <Form.ElementWrapper label={label} name={node.path}>
        <TextField
          value={(value as string) || ""}
          onChange={(e) => onChange(node.path, e.target.value)}
          disabled={disabled}
          error={!!error}
          helperText={helperText}
          fullWidth
          size="small"
          placeholder={node.schema.default ? String(node.schema.default) : ""}
        />
      </Form.ElementWrapper>
    </Stack>
  );
};

export const BooleanFieldRenderer: React.FC<FieldRendererProps> = ({
  node,
  value,
  onChange,
  error,
  disabled,
}) => {
  const label = fieldLabel(node);
  const boolValue = value === true || value === "true";

  return (
    <Stack spacing={0.75} pb={2}>
      <Form.ElementWrapper label={label} name={node.path}>
        <Stack direction="row" alignItems="center" spacing={1}>
          <Switch
            checked={boolValue}
            onChange={(e) => onChange(node.path, e.target.checked)}
            disabled={disabled}
          />
          {node.schema.description && (
            <Typography variant="caption" color="text.secondary">
              {node.schema.description}
            </Typography>
          )}
        </Stack>
        {error && (
          <Typography variant="caption" color="error">
            {error}
          </Typography>
        )}
      </Form.ElementWrapper>
    </Stack>
  );
};

export const NumberFieldRenderer: React.FC<FieldRendererProps> = ({
  node,
  value,
  onChange,
  error,
  disabled,
}) => {
  const label = fieldLabel(node);
  const isInteger = node.schema.type === "integer";
  const helperText = error || node.schema.description;

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const raw = e.target.value;
    if (raw === "") {
      onChange(node.path, "");
      return;
    }
    const parsed = isInteger ? parseInt(raw, 10) : parseFloat(raw);
    if (!Number.isNaN(parsed)) onChange(node.path, parsed);
  };

  return (
    <Stack spacing={0.75} pb={2}>
      <Form.ElementWrapper label={label} name={node.path}>
        <TextField
          type="number"
          value={value !== undefined && value !== "" ? String(value) : ""}
          onChange={handleChange}
          disabled={disabled}
          error={!!error}
          helperText={helperText}
          fullWidth
          size="small"
          placeholder={
            node.schema.default !== undefined ? String(node.schema.default) : ""
          }
          slotProps={{
            htmlInput: {
              min: node.schema.minimum,
              max: node.schema.maximum,
              step: isInteger ? 1 : "any",
            },
          }}
        />
      </Form.ElementWrapper>
    </Stack>
  );
};

export const SimpleArrayFieldRenderer: React.FC<FieldRendererProps> = ({
  node,
  value,
  onChange,
  error,
  disabled,
}) => {
  const label = fieldLabel(node);
  const itemType = node.schema.items?.type;
  const arrayValue = Array.isArray(value)
    ? (value as Array<string | number>).map(String)
    : [];

  const handleArrayChange = (newVal: string[]) => {
    if (itemType === "number" || itemType === "integer") {
      const parsed = newVal.map((v) => {
        const n = Number(v);
        return Number.isNaN(n) ? v : n;
      });
      onChange(node.path, parsed);
      return;
    }
    onChange(node.path, newVal);
  };

  return (
    <Stack spacing={0.75} pb={2}>
      <Form.ElementWrapper label={label} name={node.path}>
        <SimpleTagInput
          placeholder="Type a value and press Enter"
          value={arrayValue}
          onChange={handleArrayChange}
          helperText={error || node.schema.description}
          error={!!error}
          disabled={disabled}
        />
      </Form.ElementWrapper>
    </Stack>
  );
};

export const KeyValueFieldRenderer: React.FC<FieldRendererProps> = ({
  node,
  value,
  onChange,
  disabled,
}) => {
  const label = fieldLabel(node);
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");

  const objectValue =
    value && typeof value === "object" && !Array.isArray(value)
      ? (value as Record<string, string>)
      : {};
  const entries = Object.entries(objectValue);

  const handleAdd = () => {
    if (!newKey.trim()) return;
    onChange(node.path, { ...objectValue, [newKey.trim()]: newValue });
    setNewKey("");
    setNewValue("");
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleAdd();
    }
  };

  return (
    <Stack spacing={0.75} pb={2}>
      <Form.ElementWrapper label={label} name={node.path}>
        <Stack spacing={1}>
          {entries.map(([k, v]) => (
            <Stack key={k} direction="row" spacing={1} alignItems="center">
              <TextField
                value={k}
                disabled={disabled}
                size="small"
                sx={{ width: "40%" }}
              />
              <TextField
                value={v}
                onChange={(e) =>
                  onChange(node.path, { ...objectValue, [k]: e.target.value })
                }
                sx={{ width: "40%" }}
                disabled={disabled}
                size="small"
              />
              <IconButton
                size="small"
                color="error"
                onClick={() => {
                  const upd = { ...objectValue };
                  delete upd[k];
                  onChange(node.path, upd);
                }}
                disabled={disabled}
              >
                <Trash2 size={14} />
              </IconButton>
            </Stack>
          ))}
          <Stack direction="row" spacing={1} alignItems="center">
            <TextField
              value={newKey}
              onChange={(e) => setNewKey(e.target.value)}
              placeholder="Key"
              disabled={disabled}
              size="small"
              sx={{ width: "40%" }}
              onKeyDown={handleKeyDown}
            />
            <TextField
              value={newValue}
              onChange={(e) => setNewValue(e.target.value)}
              placeholder="Value"
              disabled={disabled}
              size="small"
              sx={{ width: "40%" }}
              onKeyDown={handleKeyDown}
            />
            <Button
              variant="outlined"
              size="small"
              onClick={handleAdd}
              disabled={disabled || !newKey.trim()}
              startIcon={<Plus size={14} />}
            >
              Add
            </Button>
          </Stack>
          {node.schema.description && (
            <Typography variant="caption" color="text.secondary">
              {node.schema.description}
            </Typography>
          )}
        </Stack>
      </Form.ElementWrapper>
    </Stack>
  );
};

// ---------------------------------------------------------------------------
// Renderer selector
// ---------------------------------------------------------------------------

export const getFieldRenderer = (
  node: FieldRendererProps["node"],
): React.FC<FieldRendererProps> | null => {
  const { schema } = node;

  switch (schema.type) {
    case "string":
      return StringFieldRenderer;
    case "boolean":
      return BooleanFieldRenderer;
    case "number":
    case "integer":
      return NumberFieldRenderer;
    case "array":
      if (
        schema.items?.type === "string" ||
        schema.items?.type === "number" ||
        schema.items?.type === "integer"
      ) {
        return SimpleArrayFieldRenderer;
      }
      return null;
    case "object":
      if (schema.additionalProperties && !schema.properties) {
        return KeyValueFieldRenderer;
      }
      return null;
    default:
      return StringFieldRenderer;
  }
};

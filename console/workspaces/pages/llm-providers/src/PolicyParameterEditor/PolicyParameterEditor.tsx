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

import React, { useState, useCallback, useEffect, useMemo } from "react";
import { Box, Button, Stack, Typography } from "@wso2/oxygen-ui";
import {
  ParameterSchema,
  ParameterValues,
  PolicyDefinition,
  ValidationError,
} from "./types";
import {
  initializeDefaultValues,
  setValueByPath,
  getValueByPath,
  createDefaultArrayItem,
  coerceValuesToSchemaTypes,
} from "./schemaUtils";
import SchemaTree from "./SchemaTree";
import { Plus } from "@wso2/oxygen-ui-icons-react";

const MAX_DESCRIPTION_LINES = 5;

const TruncatedDescription: React.FC<{ text: string }> = ({ text }) => {
  const [expanded, setExpanded] = useState(false);
  const lines = text.trim().split("\n");
  const needsTruncation = lines.length > MAX_DESCRIPTION_LINES;
  const displayed = expanded ? lines : lines.slice(0, MAX_DESCRIPTION_LINES);

  return (
    <>
      <Typography
        variant="body2"
        color="text.secondary"
        component="div"
        sx={{ whiteSpace: "pre-wrap" }}
      >
        {displayed.join("\n")}
      </Typography>
      {needsTruncation && (
        <Typography
          variant="caption"
          color="primary"
          sx={{ cursor: "pointer" }}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? "Show less" : "Show more"}
        </Typography>
      )}
    </>
  );
};

function validateRequiredFields(
  schema: ParameterSchema,
  values: ParameterValues,
  parentPath: string = "",
): ValidationError[] {
  const errors: ValidationError[] = [];

  if (schema.type === "object" && schema.properties) {
    const required = schema.required || [];
    Object.entries(schema.properties).forEach(([key, propSchema]) => {
      const path = parentPath ? `${parentPath}.${key}` : key;
      const value = getValueByPath(values, path);

      if (required.includes(key)) {
        if (
          value === undefined ||
          value === null ||
          value === "" ||
          (Array.isArray(value) && value.length === 0)
        ) {
          errors.push({ path, message: "This field is required" });
        }
      }

      if (propSchema.type === "object" && propSchema.properties) {
        errors.push(...validateRequiredFields(propSchema, values, path));
      }

      if (
        propSchema.type === "array" &&
        propSchema.items &&
        Array.isArray(value)
      ) {
        value.forEach((_, index) => {
          const itemPath = `${path}.${index}`;
          if (propSchema.items!.type === "object") {
            errors.push(
              ...validateRequiredFields(propSchema.items!, values, itemPath),
            );
          }
        });
      }
    });
  }

  return errors;
}

function isLevelOneValid(
  schema: ParameterSchema,
  values: ParameterValues,
): boolean {
  if (schema.type !== "object" || !schema.properties) return true;
  for (const key of schema.required || []) {
    const v = getValueByPath(values, key);
    if (v === undefined || v === null || v === "") return false;
    if (Array.isArray(v) && v.length === 0) return false;
  }
  return true;
}

interface PolicyParameterEditorProps {
  policyDefinition: PolicyDefinition;
  policyDisplayName?: string;
  existingValues?: ParameterValues;
  onCancel: () => void;
  onSubmit: (values: ParameterValues) => void;
  isEditMode?: boolean;
  disabled?: boolean;
}

const PolicyParameterEditor: React.FC<PolicyParameterEditorProps> = ({
  policyDefinition,
  policyDisplayName,
  existingValues,
  onCancel,
  onSubmit,
  isEditMode = false,
  disabled = false,
}) => {
  const { name, description, parameters } = policyDefinition;
  const displayName = policyDisplayName || name;

  const [values, setValues] = useState<ParameterValues>(() =>
    initializeDefaultValues(parameters, existingValues),
  );
  const [errors, setErrors] = useState<Record<string, string>>({});

  const levelOneValid = useMemo(
    () => isLevelOneValid(parameters, values),
    [parameters, values],
  );

  useEffect(() => {
    if (existingValues) {
      setValues(initializeDefaultValues(parameters, existingValues));
    }
  }, [existingValues, parameters]);

  const handleChange = useCallback((path: string, value: unknown) => {
    setValues((prev) => setValueByPath(prev, path, value));
    setErrors((prev) => {
      if (!prev[path]) return prev;
      const next = { ...prev };
      delete next[path];
      return next;
    });
  }, []);

  const handleAddArrayItem = useCallback(
    (arrayPath: string, itemSchema: ParameterSchema) => {
      setValues((prev) => {
        const current =
          (getValueByPath(prev, arrayPath) as unknown[]) || [];
        return setValueByPath(prev, arrayPath, [
          ...current,
          createDefaultArrayItem(itemSchema),
        ]);
      });
    },
    [],
  );

  const handleDeleteArrayItem = useCallback(
    (arrayPath: string, index: number) => {
      setValues((prev) => {
        const current =
          (getValueByPath(prev, arrayPath) as unknown[]) || [];
        return setValueByPath(
          prev,
          arrayPath,
          current.filter((_, i) => i !== index),
        );
      });
    },
    [],
  );

  const handleSubmit = useCallback(() => {
    const validationErrors = validateRequiredFields(parameters, values);
    if (validationErrors.length > 0) {
      const errMap: Record<string, string> = {};
      validationErrors.forEach((e) => { errMap[e.path] = e.message; });
      setErrors(errMap);
      return;
    }
    setErrors({});
    onSubmit(coerceValuesToSchemaTypes(parameters, values));
  }, [parameters, values, onSubmit]);

  return (
    <Stack spacing={2.5}>
      <Box>
        <Typography variant="h6" gutterBottom>
          {displayName}
        </Typography>
        {description && <TruncatedDescription text={description} />}
      </Box>

      <SchemaTree
        schema={parameters}
        values={values}
        onChange={handleChange}
        onAddArrayItem={handleAddArrayItem}
        onDeleteArrayItem={handleDeleteArrayItem}
        errors={errors}
        disabled={disabled}
      />

      <Stack direction="row" justifyContent="flex-end" spacing={1}>
        <Button
          variant="outlined"
          onClick={onCancel}
          disabled={disabled}
        >
          Cancel
        </Button>
        <Button
          variant="contained"
          color="primary"
          onClick={handleSubmit}
          disabled={disabled || !levelOneValid}
          endIcon={<Plus size={16} />}
        >
          {isEditMode ? "Save" : "Add"}
        </Button>
      </Stack>
    </Stack>
  );
};

export default PolicyParameterEditor;

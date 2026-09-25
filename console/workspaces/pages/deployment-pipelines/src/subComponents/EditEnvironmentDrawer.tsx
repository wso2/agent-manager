/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Collapse,
  Form,
  FormControl,
  FormControlLabel,
  FormLabel,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  useFormValidation,
} from "@agent-management-platform/views";
import { useUpdateEnvironment } from "@agent-management-platform/api-client";
import { type Environment, INPUT_LIMITS } from "@agent-management-platform/types";
import {
  editEnvironmentSchema,
  ENVIRONMENT_DISPLAY_NAME_MAX_LENGTH,
  type EditEnvironmentFormValues,
} from "../form/environmentSchema";

interface EditEnvironmentDrawerProps {
  open: boolean;
  onClose: () => void;
  environment: Environment;
  orgId: string;
}

export function EditEnvironmentDrawer({
  open,
  onClose,
  environment,
  orgId,
}: EditEnvironmentDrawerProps) {
  const [formData, setFormData] = useState<EditEnvironmentFormValues>({
    displayName: environment.displayName ?? "",
    description: "",
    isProduction: environment.isProduction,
  });

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditEnvironmentFormValues>(editEnvironmentSchema);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<Partial<Record<keyof EditEnvironmentFormValues, string>>>({});

  const {
    mutateAsync: updateEnvironment,
    isPending: isUpdating,
    error: updateError,
    reset: resetMutation,
  } = useUpdateEnvironment();

  useEffect(() => {
    if (open) {
      setFormData({
        displayName: environment.displayName ?? "",
        description: "",
        isProduction: environment.isProduction,
      });
      setLastSubmittedValidationErrors({});
      resetMutation();
    }
  }, [open, environment, resetMutation]);

  const handleFieldChange = useCallback(
    (field: keyof EditEnvironmentFormValues, value: string | boolean) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value } as EditEnvironmentFormValues;
        const fieldError = validateField(field, next[field], next);
        setFieldError(field, fieldError);
        return next;
      });
    },
    [setFieldError, validateField],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const result = editEnvironmentSchema.safeParse(formData);
      if (!result.success) {
        const fieldErrors: Partial<Record<keyof EditEnvironmentFormValues, string>> = {};
        result.error.issues.forEach((issue) => {
          if (issue.path[0]) {
            fieldErrors[issue.path[0] as keyof EditEnvironmentFormValues] = issue.message;
          }
        });
        setLastSubmittedValidationErrors(fieldErrors);
        validateForm(formData);
        return;
      }
      setLastSubmittedValidationErrors({});

      try {
        await updateEnvironment({
          params: { orgName: orgId, envName: environment.name },
          body: {
            displayName: result.data.displayName.trim(),
            description: result.data.description ?? undefined,
            isProduction: result.data.isProduction,
          },
        });
        onClose();
      } catch {
        // handled by updateError
      }
    },
    [formData, validateForm, updateEnvironment, environment, onClose, orgId],
  );

  const errorMessage = useMemo(() => {
    if (updateError) {
      return (updateError as Error)?.message ?? "Failed to update environment";
    }
    return null;
  }, [updateError]);

  const validationErrorsList = Object.values(lastSubmittedValidationErrors).filter(Boolean);

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<Edit size={24} />} title="Edit Environment" onClose={onClose} />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {errorMessage && (
              <Alert severity="error">
                <Typography variant="body2">{errorMessage}</Typography>
              </Alert>
            )}

            <Collapse in={validationErrorsList.length > 0} timeout="auto" unmountOnExit>
              <Alert severity="error" sx={{ mb: 2 }}>
                {validationErrorsList.map((error, index) => (
                  <Box key={index}>{error}</Box>
                ))}
              </Alert>
            </Collapse>

            <Form.Section>
              <Form.Header>Environment Details</Form.Header>
              <Form.Stack spacing={2}>
                <FormControl fullWidth error={Boolean(errors.displayName)}>
                  <FormLabel required>Display Name</FormLabel>
                  <TextField
                    slotProps={{ htmlInput: { maxLength: ENVIRONMENT_DISPLAY_NAME_MAX_LENGTH } }}
                    fullWidth
                    size="small"
                    value={formData.displayName}
                    onChange={(e) => handleFieldChange("displayName", e.target.value)}
                    placeholder="e.g., Production Environment"
                    error={Boolean(errors.displayName)}
                    helperText={errors.displayName}
                    disabled={isUpdating}
                  />
                </FormControl>
                <FormControl fullWidth>
                  <FormLabel>Description</FormLabel>
                  <TextField
                    slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.DESCRIPTION } }}
                    fullWidth
                    size="small"
                    multiline
                    rows={2}
                    value={formData.description ?? ""}
                    onChange={(e) => handleFieldChange("description", e.target.value)}
                    placeholder="Optional description"
                    disabled={isUpdating}
                  />
                </FormControl>
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={formData.isProduction ?? false}
                      onChange={(e) => handleFieldChange("isProduction", e.target.checked)}
                      disabled={isUpdating}
                    />
                  }
                  label="Production environment"
                />
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button variant="outlined" color="inherit" onClick={onClose} disabled={isUpdating}>
                Cancel
              </Button>
              <Button type="submit" variant="contained" color="primary" disabled={isUpdating}>
                {isUpdating ? "Saving..." : "Save"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}

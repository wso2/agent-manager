/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Collapse,
  Form,
  FormControl,
  FormLabel,
  Stack,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { getErrorMessage } from "@agent-management-platform/shared-component";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
  useFormValidation,
} from "@agent-management-platform/views";
import type {
  LLMProviderResponse,
  UpdateLLMProviderRequest,
} from "@agent-management-platform/types";
import {
  editLLMProviderSchema,
  type EditLLMProviderFormValues,
} from "../form/schema";

interface EditLLMProviderDrawerProps {
  open: boolean;
  onClose: () => void;
  provider: LLMProviderResponse;
  onUpdate: (fields: UpdateLLMProviderRequest) => Promise<LLMProviderResponse>;
  isUpdating: boolean;
}

export function EditLLMProviderDrawer({
  open,
  onClose,
  provider,
  onUpdate,
  isUpdating,
}: EditLLMProviderDrawerProps) {
  const [formData, setFormData] = useState<EditLLMProviderFormValues>({
    name: provider.name ?? "",
    description: provider.description ?? "",
  });
  const [saveError, setSaveError] = useState<string | null>(null);

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditLLMProviderFormValues>(editLLMProviderSchema);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<Partial<Record<keyof EditLLMProviderFormValues, string>>>({});

  useEffect(() => {
    if (open) {
      setFormData({
        name: provider.name ?? "",
        description: provider.description ?? "",
      });
      setLastSubmittedValidationErrors({});
      setSaveError(null);
    }
  }, [provider, open]);

  const handleFieldChange = useCallback(
    (field: keyof EditLLMProviderFormValues, value: string) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value };
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
      const result = editLLMProviderSchema.safeParse(formData);
      if (!result.success) {
        const fieldErrors: Partial<Record<keyof EditLLMProviderFormValues, string>> = {};
        result.error.issues.forEach((issue) => {
          if (issue.path[0]) {
            fieldErrors[issue.path[0] as keyof EditLLMProviderFormValues] =
              issue.message;
          }
        });
        setLastSubmittedValidationErrors(fieldErrors);
        validateForm(formData); // syncs errors to form fields
        return;
      }
      setLastSubmittedValidationErrors({});
      setSaveError(null);

      try {
        await onUpdate({
          name: formData.name.trim(),
          description: formData.description?.trim() || undefined,
        });
        onClose();
      } catch (err) {
        setSaveError(getErrorMessage(err) || "Failed to update LLM provider");
      }
    },
    [formData, validateForm, onUpdate, onClose],
  );

  const validationErrorsList = Object.values(lastSubmittedValidationErrors).filter(
    Boolean,
  );
  const hasValidationErrors = validationErrorsList.length > 0;

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit LLM Provider"
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {saveError && (
              <Alert severity="error">
                <Typography variant="body2">{saveError}</Typography>
              </Alert>
            )}

            <Collapse in={hasValidationErrors} timeout="auto" unmountOnExit>
              <Alert severity="error" sx={{ mb: 2 }}>
                {validationErrorsList.map((error, index) => (
                  <Box key={index}>{error}</Box>
                ))}
              </Alert>
            </Collapse>

            <Form.Section>
              <Form.Header>Provider Details</Form.Header>
              <Form.Stack spacing={2}>
                <FormControl fullWidth error={Boolean(errors.name)}>
                  <FormLabel required>Name</FormLabel>
                  <TextField
                    fullWidth
                    size="small"
                    value={formData.name}
                    onChange={(e) => handleFieldChange("name", e.target.value)}
                    placeholder="e.g., Production OpenAI"
                    error={Boolean(errors.name)}
                    helperText={errors.name}
                    disabled={isUpdating}
                  />
                </FormControl>
                <FormControl fullWidth error={Boolean(errors.description)}>
                  <FormLabel>Description</FormLabel>
                  <TextField
                    fullWidth
                    multiline
                    minRows={2}
                    size="small"
                    value={formData.description}
                    onChange={(e) =>
                      handleFieldChange("description", e.target.value)
                    }
                    placeholder="Optional description"
                    error={Boolean(errors.description)}
                    helperText={errors.description}
                    disabled={isUpdating}
                  />
                </FormControl>
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
                color="inherit"
                onClick={onClose}
                disabled={isUpdating}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={isUpdating}
              >
                {isUpdating ? "Saving..." : "Save"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}

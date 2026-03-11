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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Collapse,
  Form,
  FormControl,
  FormLabel,
  Stack,
  Switch,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerWrapper,
  DrawerHeader,
  DrawerContent,
  useFormValidation,
} from "@agent-management-platform/views";
import { useUpdateGateway } from "@agent-management-platform/api-client";
import type {
  GatewayResponse,
  UpdateGatewayRequest,
} from "@agent-management-platform/types";
import { editGatewaySchema, type EditGatewayFormValues } from "../form/schema";

interface EditGatewayDrawerProps {
  open: boolean;
  onClose: () => void;
  gateway: GatewayResponse;
  orgId: string;
  onSuccess?: () => void;
}

export function EditGatewayDrawer({
  open,
  onClose,
  gateway,
  orgId,
  onSuccess,
}: EditGatewayDrawerProps) {
  const [formData, setFormData] = useState<EditGatewayFormValues>({
    displayName: gateway.displayName,
    isCritical: gateway.isCritical,
  });

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditGatewayFormValues>(editGatewaySchema);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<Partial<Record<keyof EditGatewayFormValues, string>>>({});

  const {
    mutateAsync: updateGateway,
    isPending: isUpdating,
    error: updateError,
  } = useUpdateGateway();

  const isPending = isUpdating;

  useEffect(() => {
    if (open) {
      setFormData({
        displayName: gateway.displayName,
        isCritical: gateway.isCritical,
      });
    }
  }, [gateway, open]);

  const handleFieldChange = useCallback(
    (
      field: keyof EditGatewayFormValues,
      value: string | boolean | string[],
    ) => {
      setFormData((prev) => {
        const next = { ...prev, [field]: value } as EditGatewayFormValues;
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
      const result = editGatewaySchema.safeParse(formData);
      if (!result.success) {
        const fieldErrors: Partial<Record<keyof EditGatewayFormValues, string>> = {};
        result.error.issues.forEach((issue) => {
          if (issue.path[0]) {
            fieldErrors[issue.path[0] as keyof EditGatewayFormValues] = issue.message;
          }
        });
        setLastSubmittedValidationErrors(fieldErrors);
        validateForm(formData); // syncs errors to form fields
        return;
      }
      setLastSubmittedValidationErrors({});

      const payload: UpdateGatewayRequest = {
        displayName: formData.displayName.trim(),
        isCritical: formData.isCritical,
      };

      try {
        await updateGateway({
          params: { orgName: orgId, gatewayId: gateway.uuid },
          body: payload,
        });
        onClose();
        onSuccess?.();
      } catch {
        // Error is handled by updateError from useUpdateGateway
      }
    },
    [formData, validateForm, updateGateway, orgId, gateway.uuid, onClose, onSuccess],
  );

  const errorMessage = useMemo(() => {
    if (updateError) {
      return (updateError as Error)?.message ?? "Failed to update gateway";
    }
    return null;
  }, [updateError]);

  const validationErrorsList = Object.values(lastSubmittedValidationErrors).filter(Boolean);
  const hasValidationErrors = validationErrorsList.length > 0;

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit AI Gateway"
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Stack spacing={3}>
            {errorMessage && (
              <Alert severity="error">
                <Typography variant="body2">{errorMessage}</Typography>
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
              <Form.Header>Gateway Details</Form.Header>
              <Form.Stack spacing={2}>
                <FormControl fullWidth error={Boolean(errors.displayName)}>
                  <FormLabel required>Display Name</FormLabel>
                  <TextField
                    fullWidth
                    size="small"
                    value={formData.displayName}
                    onChange={(e) =>
                      handleFieldChange("displayName", e.target.value)
                    }
                    placeholder="e.g., Production AI Gateway"
                    error={Boolean(errors.displayName)}
                    helperText={errors.displayName}
                    disabled={isPending}
                  />
                </FormControl>

                <FormControl fullWidth>
                  <FormLabel>Critical production gateway</FormLabel>
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <Switch
                      checked={formData.isCritical}
                      onChange={(_, checked) =>
                        handleFieldChange("isCritical", checked)
                      }
                      disabled={isPending}
                    />
                    <Typography variant="caption" color="text.secondary">
                      Mark as critical for production deployments
                    </Typography>
                  </Stack>
                </FormControl>
              </Form.Stack>
            </Form.Section>

            <Box display="flex" justifyContent="flex-end" gap={1} mt={2}>
              <Button
                variant="outlined"
                color="inherit"
                onClick={onClose}
                disabled={isPending}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                color="primary"
                disabled={isPending}
              >
                {isPending ? "Saving..." : "Save"}
              </Button>
            </Box>
          </Stack>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}

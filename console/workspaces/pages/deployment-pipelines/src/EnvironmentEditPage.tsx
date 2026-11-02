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

import React, { useCallback, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Collapse,
  Form,
  FormControlLabel,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { PageLayout, TextInput, useFormValidation } from "@agent-management-platform/views";
import { useGetEnvironment, useUpdateEnvironment } from "@agent-management-platform/api-client";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type Environment,
  INPUT_LIMITS,
} from "@agent-management-platform/types";
import {
  editEnvironmentSchema,
  ENVIRONMENT_DISPLAY_NAME_MAX_LENGTH,
  type EditEnvironmentFormValues,
} from "./form/environmentSchema";

interface EditEnvironmentFormProps {
  environment: Environment;
  orgId: string;
  listHref: string;
}

function EditEnvironmentForm({ environment, orgId, listHref }: EditEnvironmentFormProps) {
  const navigate = useNavigate();
  const [formData, setFormData] = useState<EditEnvironmentFormValues>({
    displayName: environment.displayName ?? "",
    description: "",
    isProduction: environment.isProduction,
  });
  const [initialSnapshot] = useState(() => JSON.stringify(formData));

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditEnvironmentFormValues>(editEnvironmentSchema);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<Partial<Record<keyof EditEnvironmentFormValues, string>>>({});

  const {
    mutateAsync: updateEnvironment,
    isPending: isUpdating,
    error: updateError,
  } = useUpdateEnvironment();

  const isDirty = useMemo(
    () => JSON.stringify(formData) !== initialSnapshot,
    [initialSnapshot, formData],
  );
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  const handleCancel = useCallback(() => {
    allowNavigation(() => navigate(listHref));
  }, [allowNavigation, navigate, listHref]);

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
        allowNavigation(() => navigate(listHref));
      } catch {
        // handled by updateError
      }
    },
    [formData, validateForm, updateEnvironment, environment, orgId, allowNavigation, navigate,
      listHref],
  );

  const errorMessage = useMemo(() => {
    if (updateError) {
      return (updateError as Error)?.message ?? "Failed to update environment";
    }
    return null;
  }, [updateError]);

  const validationErrorsList = Object.values(lastSubmittedValidationErrors).filter(Boolean);

  return (
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

        <Form.Stack spacing={3}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
              <TextInput
                label="Display Name"
                maxLength={ENVIRONMENT_DISPLAY_NAME_MAX_LENGTH}
                hideCharacterCount={
                  formData.displayName.length < ENVIRONMENT_DISPLAY_NAME_MAX_LENGTH
                }
                value={formData.displayName}
                onChange={(e) => handleFieldChange("displayName", e.target.value)}
                placeholder="e.g., Production Environment"
                error={Boolean(errors.displayName)}
                helperText={errors.displayName}
                disabled={isUpdating}
              />
            </Grid>
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
              <TextInput
                label="Description (optional)"
                maxLength={INPUT_LIMITS.DESCRIPTION}
                hideCharacterCount={
                  (formData.description?.length ?? 0) < INPUT_LIMITS.DESCRIPTION
                }
                multiline
                rows={2}
                value={formData.description ?? ""}
                onChange={(e) => handleFieldChange("description", e.target.value)}
                placeholder="Optional description"
                disabled={isUpdating}
              />
            </Grid>
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
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
            </Grid>
          </Grid>
        </Form.Stack>

        <Box display="flex" gap={1} mt={2}>
          <Button variant="outlined" color="inherit" onClick={handleCancel} disabled={isUpdating}>
            Cancel
          </Button>
          <Button type="submit" variant="contained" color="primary" disabled={isUpdating}>
            {isUpdating ? "Saving..." : "Save"}
          </Button>
        </Box>
      </Stack>
    </form>
  );
}

export function EnvironmentEditPage() {
  const { orgId = "", envName = "" } = useParams<{ orgId: string; envName: string }>();
  const { data: environment, isLoading } = useGetEnvironment({ orgName: orgId, envName });
  const listHref = generatePath(absoluteRouteMap.children.org.children.environments.path, {
    orgId,
  });

  return (
    <PageLayout
      title="Edit Environment"
      disableIcon
      backHref={listHref}
      backLabel="Back to environments"
    >
      {isLoading || !environment ? (
        <Skeleton variant="rounded" width="100%" height={400} />
      ) : (
        <EditEnvironmentForm environment={environment} orgId={orgId} listHref={listHref} />
      )}
    </PageLayout>
  );
}

export default EnvironmentEditPage;

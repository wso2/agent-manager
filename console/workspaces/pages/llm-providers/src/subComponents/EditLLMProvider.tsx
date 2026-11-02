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

import React, { useCallback, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Collapse,
  Form,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  getErrorMessage,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import { PageLayout, TextInput, useFormValidation } from "@agent-management-platform/views";
import {
  useGetLLMProvider,
  useUpdateLLMProvider,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type LLMProviderResponse,
} from "@agent-management-platform/types";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  editLLMProviderSchema,
  type EditLLMProviderFormValues,
  LLM_PROVIDER_NAME_MAX_LENGTH,
  LLM_PROVIDER_DESCRIPTION_MAX_LENGTH,
} from "../form/schema";

interface EditLLMProviderFormProps {
  provider: LLMProviderResponse;
  orgId: string;
  providerId: string;
  viewHref: string;
}

function EditLLMProviderForm({
  provider,
  orgId,
  providerId,
  viewHref,
}: EditLLMProviderFormProps) {
  const navigate = useNavigate();
  const [formData, setFormData] = useState<EditLLMProviderFormValues>({
    name: provider.name ?? "",
    description: provider.description ?? "",
  });
  const [saveError, setSaveError] = useState<string | null>(null);
  const [initialSnapshot] = useState(() => JSON.stringify(formData));

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditLLMProviderFormValues>(editLLMProviderSchema);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<Partial<Record<keyof EditLLMProviderFormValues, string>>>({});

  const { mutateAsync: updateProvider, isPending: isUpdating } =
    useUpdateLLMProvider();

  const isDirty = useMemo(
    () => JSON.stringify(formData) !== initialSnapshot,
    [initialSnapshot, formData],
  );
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

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

  const handleCancel = () => allowNavigation(() => navigate(viewHref));

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
        await updateProvider({
          params: { orgName: orgId, providerId },
          body: {
            ...provider,
            name: formData.name.trim(),
            description: formData.description?.trim() || undefined,
          },
        });
        allowNavigation(() => navigate(viewHref));
      } catch (err) {
        setSaveError(getErrorMessage(err) || "Failed to update LLM provider");
      }
    },
    [formData, validateForm, updateProvider, orgId, providerId, provider,
      allowNavigation, navigate, viewHref],
  );

  const validationErrorsList = Object.values(lastSubmittedValidationErrors).filter(
    Boolean,
  );
  const hasValidationErrors = validationErrorsList.length > 0;

  return (
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

        <Form.Stack spacing={3}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
              <TextInput
                label="Name"
                maxLength={LLM_PROVIDER_NAME_MAX_LENGTH}
                hideCharacterCount={
                  formData.name.length < LLM_PROVIDER_NAME_MAX_LENGTH
                }
                value={formData.name}
                onChange={(e) => handleFieldChange("name", e.target.value)}
                placeholder="e.g., Production OpenAI"
                error={Boolean(errors.name)}
                helperText={errors.name}
                disabled={isUpdating}
              />
            </Grid>
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
              <TextInput
                label="Description (optional)"
                multiline
                minRows={2}
                maxLength={LLM_PROVIDER_DESCRIPTION_MAX_LENGTH}
                hideCharacterCount={
                  (formData.description?.length ?? 0) <
                  LLM_PROVIDER_DESCRIPTION_MAX_LENGTH
                }
                value={formData.description}
                onChange={(e) =>
                  handleFieldChange("description", e.target.value)
                }
                placeholder="Optional description"
                error={Boolean(errors.description)}
                helperText={errors.description}
                disabled={isUpdating}
              />
            </Grid>
          </Grid>
        </Form.Stack>

        <Box display="flex" gap={1} mt={2}>
          <Button
            variant="outlined"
            color="inherit"
            onClick={handleCancel}
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
  );
}

export function EditLLMProvider() {
  const { orgId = "", providerId = "" } = useParams<{
    orgId: string;
    providerId: string;
  }>();
  const { data: provider, isLoading } = useGetLLMProvider({
    orgName: orgId,
    providerId,
  });
  const viewHref = generatePath(
    absoluteRouteMap.children.org.children.llmProviders.children.view.path,
    { orgId, providerId },
  );

  return (
    <PageLayout
      title="Edit LLM Provider"
      disableIcon
      backHref={viewHref}
      backLabel="Back to LLM provider"
    >
      {isLoading || !provider ? (
        <Skeleton variant="rounded" width="100%" height={300} />
      ) : (
        <EditLLMProviderForm
          provider={provider}
          orgId={orgId}
          providerId={providerId}
          viewHref={viewHref}
        />
      )}
    </PageLayout>
  );
}

export default EditLLMProvider;

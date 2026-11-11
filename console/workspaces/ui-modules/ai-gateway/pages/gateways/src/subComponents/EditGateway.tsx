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
  PageLayout,
  TextInput,
  useFormValidation,
  LoadErrorNotice,
} from "@agent-management-platform/views";
import {
  useGetGateway,
  useUpdateGateway,
  useUpdateSelfHostedGateway,
} from "@agent-management-platform/api-client";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type GatewayResponse,
  isSelfHostedGateway,
  type UpdateGatewayRequest,
} from "@agent-management-platform/types";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { editGatewaySchema, type EditGatewayFormValues, GATEWAY_DISPLAY_NAME_MAX_LENGTH } from "../form/schema";

interface EditGatewayFormProps {
  gateway: GatewayResponse;
  orgId: string;
  viewHref: string;
}

function EditGatewayForm({ gateway, orgId, viewHref }: EditGatewayFormProps) {
  const navigate = useNavigate();
  const [formData, setFormData] = useState<EditGatewayFormValues>({
    displayName: gateway.displayName,
    isCritical: gateway.isCritical,
  });
  const [initialSnapshot] = useState(() => JSON.stringify(formData));

  const { errors, validateForm, setFieldError, validateField } =
    useFormValidation<EditGatewayFormValues>(editGatewaySchema);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<Partial<Record<keyof EditGatewayFormValues, string>>>({});

  const isSelfHosted = isSelfHostedGateway(gateway);

  const managedUpdate = useUpdateGateway();
  const selfHostedUpdate = useUpdateSelfHostedGateway();
  const {
    mutateAsync: updateGateway,
    isPending: isUpdating,
    error: updateError,
  } = isSelfHosted ? selfHostedUpdate : managedUpdate;

  const isPending = isUpdating;

  const isDirty = useMemo(
    () => JSON.stringify(formData) !== initialSnapshot,
    [initialSnapshot, formData],
  );
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  const handleCancel = useCallback(() => {
    allowNavigation(() => navigate(viewHref));
  }, [allowNavigation, navigate, viewHref]);

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
        allowNavigation(() => navigate(viewHref));
      } catch {
        // Error is handled by updateError from useUpdateGateway
      }
    },
    [
      formData, validateForm, updateGateway, orgId, gateway.uuid,
      allowNavigation, navigate, viewHref,
    ],
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

        <Form.Stack spacing={3}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Display Name"
                maxLength={GATEWAY_DISPLAY_NAME_MAX_LENGTH}
                hideCharacterCount={
                  formData.displayName.length < GATEWAY_DISPLAY_NAME_MAX_LENGTH
                }
                value={formData.displayName}
                onChange={(e) =>
                  handleFieldChange("displayName", e.target.value)
                }
                placeholder="e.g., Production Gateway"
                error={Boolean(errors.displayName)}
                helperText={errors.displayName}
                disabled={isPending}
              />
            </Grid>
          </Grid>
        </Form.Stack>

        <Box display="flex" gap={1} mt={2}>
          <Button
            variant="outlined"
            color="inherit"
            onClick={handleCancel}
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
  );
}

export function EditGateway() {
  const { orgId = "", gatewayId = "" } = useParams<{ orgId: string; gatewayId: string }>();
  const { data: gateway, isLoading, error, refetch } = useGetGateway({ orgName: orgId, gatewayId });
  // Edit is opened from the gateway's view page, so Back/Cancel/Save return there.
  const viewHref = generatePath(
    absoluteRouteMap.children.org.children.gateways.children.view.path,
    { orgId, gatewayId },
  );

  return (
    <PageLayout
      title="Edit Gateway"
      disableIcon
      backHref={viewHref}
      backLabel="Back to gateway"
    >
      {isLoading ? (
        <Skeleton variant="rounded" width="100%" height={240} />
      ) : !gateway ? (
        <LoadErrorNotice
          entityLabel="gateway"
          error={error}
          onRetry={() => refetch()}
          backHref={generatePath(absoluteRouteMap.children.org.children.gateways.path, { orgId })}
          backLabel="Back to gateways"
        />
      ) : (
        <EditGatewayForm gateway={gateway} orgId={orgId} viewHref={viewHref} />
      )}
    </PageLayout>
  );
}

export default EditGateway;

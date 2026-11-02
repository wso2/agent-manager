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

import React, { useCallback, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Collapse,
  Form,
  Grid,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { useCreateAgentIdentityGroup } from "@agent-management-platform/api-client";
import { useFormValidation, useDirtyState, TextInput } from "@agent-management-platform/views";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import { absoluteRouteMap, INPUT_LIMITS } from "@agent-management-platform/types";
import { withSearchParams } from "../../utils/withSearchParams";
import {
  createAgentIdentityGroupSchema,
  type CreateAgentIdentityGroupFormValues,
  IDENTITY_NAME_MAX_LENGTH,
} from "./schemas";

export const GroupCreatePage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const envName = searchParams.get("envName") ?? "";
  const navigate = useNavigate();

  const [formData, setFormData] = useState<CreateAgentIdentityGroupFormValues>({
    name: "",
    description: "",
  });

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<CreateAgentIdentityGroupFormValues>(createAgentIdentityGroupSchema);
  const { isDirty, checkDirty, resetDirty } = useDirtyState(formData);
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);
  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<typeof errors>({});

  const {
    mutateAsync: createGroup,
    isPending: isCreating,
    error: createError,
  } = useCreateAgentIdentityGroup();

  const groupsNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.groups;

  const groupsPath = orgId
    ? withSearchParams(generatePath(groupsNode.path, { orgId }), searchParams)
    : "#";

  const handleFieldChange = useCallback(
    (field: keyof CreateAgentIdentityGroupFormValues, value: string) => {
      const newData = { ...formData, [field]: value };
      setFormData(newData);
      checkDirty(newData);
      setFieldError(field, validateField(field, value));
    },
    [formData, checkDirty, validateField, setFieldError],
  );

  const handleSubmit = useCallback(async () => {
    if (!orgId || !envName) return;
    if (!validateForm(formData)) {
      setLastSubmittedValidationErrors(errors);
      return;
    }
    setLastSubmittedValidationErrors({});

    try {
      const created = await createGroup({
        params: { orgName: orgId, envName },
        body: {
          name: formData.name.trim(),
          description: formData.description?.trim() || undefined,
        },
      });
      resetDirty();
      clearErrors();
      allowNavigation(() =>
        navigate(
          withSearchParams(
            generatePath(groupsNode.children.detail.path, { orgId, groupId: created.id }),
            searchParams,
          ),
        ),
      );
    } catch {
      // createError state is set by React Query and displayed in the Alert above
    }
  }, [
    formData,
    validateForm,
    errors,
    createGroup,
    orgId,
    envName,
    resetDirty,
    clearErrors,
    allowNavigation,
    navigate,
    groupsNode,
    searchParams,
  ]);

  const submitErrors = Object.values(lastSubmittedValidationErrors);

  return (
    <>
      <Box display="flex" flexDirection="column" gap={2}>
        {createError != null && (
          <Alert severity="error">
            {(createError as Error)?.message ?? "Failed to create group"}
          </Alert>
        )}

        <Form.Stack spacing={3}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 6, xl: 4 }}>
              <TextInput
                label="Name"
                maxLength={IDENTITY_NAME_MAX_LENGTH}
                hideCharacterCount={formData.name.length < IDENTITY_NAME_MAX_LENGTH}
                id="name"
                value={formData.name}
                onChange={(e) => handleFieldChange("name", e.target.value)}
                placeholder="on-call-agents"
                autoComplete="off"
                error={!!errors.name}
                helperText={errors.name}
              />
            </Grid>
            <Grid size={{ xl: 8 }} sx={{ display: { xs: "none", xl: "block" } }} />
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
              <TextInput
                label="Description (optional)"
                maxLength={INPUT_LIMITS.DESCRIPTION}
                hideCharacterCount={(formData.description?.length ?? 0) < INPUT_LIMITS.DESCRIPTION}
                id="description"
                value={formData.description}
                onChange={(e) => handleFieldChange("description", e.target.value)}
                placeholder="Describe the group's purpose"
                multiline
                minRows={2}
                maxRows={6}
                error={!!errors.description}
                helperText={errors.description}
              />
            </Grid>
          </Grid>
        </Form.Stack>

        <Box display="flex" flexDirection="column" gap={3}>
          <Collapse in={submitErrors.length > 0} timeout="auto" unmountOnExit>
            <Alert severity="error">
              {submitErrors.map((error, index) => (
                <Box key={index}>{error}</Box>
              ))}
            </Alert>
          </Collapse>
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            <Button
              variant="outlined"
              color="primary"
              onClick={() => allowNavigation(() => navigate(groupsPath))}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              color="primary"
              startIcon={<Plus size={16} />}
              onClick={handleSubmit}
              disabled={isCreating || !formData.name.trim()}
            >
              Create Group
            </Button>
          </Box>
        </Box>
      </Box>
    </>
  );
};

export default GroupCreatePage;

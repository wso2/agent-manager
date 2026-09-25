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
import { Alert, Box, Button, Collapse, Form, TextField } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import { useCreateGroup } from "@agent-management-platform/api-client";
import {
  useFormValidation,
  useDirtyState,
} from "@agent-management-platform/views";
import { absoluteRouteMap, INPUT_LIMITS } from "@agent-management-platform/types";
import {
  createGroupSchema,
  IDENTITY_NAME_MAX_LENGTH,
  type CreateGroupFormValues,
} from "./forms/schemas";

export const GroupCreatePage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const [formData, setFormData] = useState<CreateGroupFormValues>({
    name: "",
    description: "",
  });

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<CreateGroupFormValues>(createGroupSchema);
  const { checkDirty, resetDirty } = useDirtyState(formData);
  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<typeof errors>({});

  const {
    mutateAsync: createGroup,
    isPending: isCreating,
    error: createError,
  } = useCreateGroup();

  const groupsPath = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.settings.children.identities
          .children.groups.path,
        { orgId },
      )
    : "#";

  const handleFieldChange = useCallback(
    (field: keyof CreateGroupFormValues, value: string) => {
      const newData = { ...formData, [field]: value };
      setFormData(newData);
      checkDirty(newData);
      setFieldError(field, validateField(field, value));
    },
    [formData, checkDirty, validateField, setFieldError],
  );

  const handleSubmit = useCallback(async () => {
    if (!validateForm(formData)) {
      setLastSubmittedValidationErrors(errors);
      return;
    }
    setLastSubmittedValidationErrors({});

    try {
      const created = await createGroup({
        params: { orgName: orgId },
        body: {
          name: formData.name.trim(),
          description: formData.description?.trim() || undefined,
        },
      });
      resetDirty();
      clearErrors();
      navigate(
        generatePath(
          absoluteRouteMap.children.org.children.settings.children.identities
            .children.groups.children.detail.path,
          { orgId, groupId: created.id },
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
    resetDirty,
    clearErrors,
    navigate,
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
          <Form.Section>
            <Form.Subheader>Group Details</Form.Subheader>
            <Form.Stack spacing={2}>
              <Form.ElementWrapper label="Name" name="name">
                <TextField
                  slotProps={{ htmlInput: { maxLength: IDENTITY_NAME_MAX_LENGTH } }}
                  id="name"
                  value={formData.name}
                  onChange={(e) => handleFieldChange("name", e.target.value)}
                  placeholder="engineering"
                  autoComplete="off"
                  error={!!errors.name}
                  helperText={errors.name}
                  fullWidth
                />
              </Form.ElementWrapper>

              <Form.ElementWrapper
                label="Description (optional)"
                name="description"
              >
                <TextField
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.DESCRIPTION } }}
                  id="description"
                  value={formData.description}
                  onChange={(e) =>
                    handleFieldChange("description", e.target.value)
                  }
                  placeholder="Describe the group's purpose"
                  multiline
                  minRows={2}
                  maxRows={6}
                  error={!!errors.description}
                  helperText={errors.description}
                  fullWidth
                />
              </Form.ElementWrapper>
            </Form.Stack>
          </Form.Section>
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
              onClick={() => navigate(groupsPath)}
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

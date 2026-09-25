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
import {
  useFormValidation,
  useDirtyState,
  TextInput,
} from "@agent-management-platform/views";
import { useNavigate, useParams, generatePath } from "react-router-dom";
import { useCreateUser } from "@agent-management-platform/api-client";
import { absoluteRouteMap, INPUT_LIMITS } from "@agent-management-platform/types";
import { addUserSchema, type AddUserFormValues } from "./forms/schemas";

export const UserAddPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();

  const identitiesRoute =
    absoluteRouteMap.children.org.children.settings.children.identities;

  const usersPath = orgId
    ? generatePath(identitiesRoute.children.users.path, { orgId })
    : "#";

  const {
    mutateAsync: createUserMutation,
    isPending: loading,
    error: createError,
  } = useCreateUser();

  const [formData, setFormData] = useState<AddUserFormValues>({
    username: "",
    password: "",
    firstName: "",
    lastName: "",
    email: "",
  });

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<AddUserFormValues>(addUserSchema);
  const { checkDirty, resetDirty } = useDirtyState(formData);
  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<typeof errors>({});

  const handleFieldChange = useCallback(
    (field: keyof AddUserFormValues, value: string) => {
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

    // Required fields (username, password) are enforced by validateForm above,
    // so we only assemble the attributes payload here.
    const attributes: Record<string, string> = {
      username: formData.username,
      password: formData.password,
    };
    if (formData.firstName) {
      attributes.given_name = formData.firstName;
    }
    if (formData.lastName) {
      attributes.family_name = formData.lastName;
    }
    if (formData.email) {
      attributes.email = formData.email;
    }

    try {
      await createUserMutation({
        params: { orgName: orgId },
        body: {
          type: "engineer",
          attributes,
        },
      });
      resetDirty();
      clearErrors();
      navigate(usersPath);
    } catch {
      // createError state is set by React Query and displayed in the Alert above
    }
  }, [
    formData,
    validateForm,
    errors,
    createUserMutation,
    orgId,
    resetDirty,
    clearErrors,
    navigate,
    usersPath,
  ]);

  const submitErrors = Object.values(lastSubmittedValidationErrors);

  return (
    <>
      <Box display="flex" flexDirection="column" gap={2}>
        {createError != null && (
          <Alert severity="error">
            {(createError as Error)?.message ?? "Failed to create user"}
          </Alert>
        )}

        <Form.Stack spacing={3}>
          <Form.Section>
            <Form.Subheader>User Details</Form.Subheader>
            <Form.Stack spacing={2}>
              <Form.ElementWrapper label="Username" name="username">
                <TextField
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                  id="username"
                  value={formData.username}
                  onChange={(e) =>
                    handleFieldChange("username", e.target.value)
                  }
                  placeholder="jane.doe"
                  autoComplete="off"
                  disabled={loading}
                  error={!!errors.username}
                  helperText={errors.username}
                  fullWidth
                />
              </Form.ElementWrapper>

              <Form.ElementWrapper label="Password" name="password">
                <TextInput
                  maxLength={INPUT_LIMITS.PASSWORD}
                  id="password"
                  type="password"
                  showPasswordToggle
                  value={formData.password}
                  onChange={(e) =>
                    handleFieldChange("password", e.target.value)
                  }
                  autoComplete="new-password"
                  disabled={loading}
                  error={!!errors.password}
                  helperText={errors.password}
                  fullWidth
                />
              </Form.ElementWrapper>

              <Form.ElementWrapper
                label="First Name (optional)"
                name="firstName"
              >
                <TextField
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                  id="firstName"
                  value={formData.firstName}
                  onChange={(e) =>
                    handleFieldChange("firstName", e.target.value)
                  }
                  disabled={loading}
                  error={!!errors.firstName}
                  helperText={errors.firstName}
                  fullWidth
                />
              </Form.ElementWrapper>

              <Form.ElementWrapper label="Last Name (optional)" name="lastName">
                <TextField
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.NAME } }}
                  id="lastName"
                  value={formData.lastName}
                  onChange={(e) =>
                    handleFieldChange("lastName", e.target.value)
                  }
                  disabled={loading}
                  error={!!errors.lastName}
                  helperText={errors.lastName}
                  fullWidth
                />
              </Form.ElementWrapper>

              <Form.ElementWrapper
                label="Email Address (optional)"
                name="email"
              >
                <TextField
                  slotProps={{ htmlInput: { maxLength: INPUT_LIMITS.SHORT_TEXT } }}
                  id="email"
                  type="email"
                  value={formData.email}
                  onChange={(e) => handleFieldChange("email", e.target.value)}
                  placeholder="user@example.com"
                  disabled={loading}
                  error={!!errors.email}
                  helperText={errors.email}
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
              onClick={() => navigate(usersPath)}
              disabled={loading}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              color="primary"
              startIcon={<Plus size={16} />}
              onClick={handleSubmit}
              disabled={
                loading || !formData.username.trim() || !formData.password
              }
            >
              Create User
            </Button>
          </Box>
        </Box>
      </Box>
    </>
  );
};

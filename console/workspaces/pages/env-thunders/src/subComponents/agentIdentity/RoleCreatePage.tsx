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
  Autocomplete,
  Box,
  Button,
  Chip,
  Collapse,
  Form,
  TextField,
} from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  useCreateAgentIdentityRole,
  useListAgentIdentityScopes,
} from "@agent-management-platform/api-client";
import { useFormValidation, useDirtyState } from "@agent-management-platform/views";
import { absoluteRouteMap, INPUT_LIMITS } from "@agent-management-platform/types";
import { withSearchParams } from "../../utils/withSearchParams";
import {
  createAgentIdentityRoleSchema,
  type CreateAgentIdentityRoleFormValues,
  IDENTITY_NAME_MAX_LENGTH,
} from "./schemas";
import type { ScopeChoice } from "./scopeChoice";

export const RoleCreatePage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const envName = searchParams.get("envName") ?? "";
  const navigate = useNavigate();

  const [formData, setFormData] = useState<CreateAgentIdentityRoleFormValues>({
    name: "",
    description: "",
  });
  const [selectedScopes, setSelectedScopes] = useState<ScopeChoice[]>([]);

  const { data: scopesData } = useListAgentIdentityScopes({
    orgName: orgId,
    envName,
  });
  const scopes = scopesData?.scopes ?? [];

  const { errors, validateField, validateForm, clearErrors, setFieldError } =
    useFormValidation<CreateAgentIdentityRoleFormValues>(createAgentIdentityRoleSchema);
  const { checkDirty, resetDirty } = useDirtyState(formData);
  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] =
    useState<typeof errors>({});

  const {
    mutateAsync: createRole,
    isPending: isCreating,
    error: createError,
  } = useCreateAgentIdentityRole();

  const rolesNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.roles;

  const rolesPath = orgId
    ? withSearchParams(generatePath(rolesNode.path, { orgId }), searchParams)
    : "#";

  const handleFieldChange = useCallback(
    (field: keyof CreateAgentIdentityRoleFormValues, value: string) => {
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
      const created = await createRole({
        params: { orgName: orgId, envName },
        body: {
          name: formData.name.trim(),
          description: formData.description?.trim() || undefined,
          scopes: selectedScopes.map((s) => s.scope),
        },
      });
      resetDirty();
      clearErrors();
      navigate(
        withSearchParams(
          generatePath(rolesNode.children.detail.path, { orgId, roleId: created.id }),
          searchParams,
        ),
      );
    } catch {
      // createError state is set by React Query and displayed in the Alert above
    }
  }, [
    formData,
    validateForm,
    errors,
    createRole,
    orgId,
    envName,
    selectedScopes,
    resetDirty,
    clearErrors,
    navigate,
    rolesNode,
    searchParams,
  ]);

  const submitErrors = Object.values(lastSubmittedValidationErrors);

  return (
    <>
      <Box display="flex" flexDirection="column" gap={2}>
        {createError != null && (
          <Alert severity="error">
            {(createError as Error)?.message ?? "Failed to create role"}
          </Alert>
        )}

        <Form.Stack spacing={3}>
          <Form.Section>
            <Form.Subheader>Role Details</Form.Subheader>
            <Form.Stack spacing={2}>
              <Form.ElementWrapper label="Name" name="name">
                <TextField
                  slotProps={{ htmlInput: { maxLength: IDENTITY_NAME_MAX_LENGTH } }}
                  id="name"
                  value={formData.name}
                  onChange={(e) => handleFieldChange("name", e.target.value)}
                  placeholder="agent-admin"
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
                  placeholder="Describe the role's purpose and permissions"
                  multiline
                  minRows={2}
                  maxRows={6}
                  error={!!errors.description}
                  helperText={errors.description}
                  fullWidth
                />
              </Form.ElementWrapper>

              <Form.ElementWrapper label="Scopes (optional)" name="scopes">
                <Autocomplete
                  id="scopes"
                  multiple
                  disableCloseOnSelect
                  options={scopes}
                  value={selectedScopes}
                  onChange={(_e, value) => setSelectedScopes(value as ScopeChoice[])}
                  getOptionLabel={(option) => (option as ScopeChoice).scope}
                  isOptionEqualToValue={(option, value) =>
                    (option as ScopeChoice).scope === (value as ScopeChoice).scope
                  }
                  renderTags={(value, getTagProps) =>
                    value.map((option, index) => (
                      <Chip
                        {...getTagProps({ index })}
                        key={option.scope}
                        label={option.scope}
                        size="small"
                      />
                    ))
                  }
                  renderInput={(params) => (
                    <TextField {...params} placeholder="Search scopes..." />
                  )}
                  noOptionsText="No scopes in the catalog"
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
              onClick={() => navigate(rolesPath)}
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
              Create Role
            </Button>
          </Box>
        </Box>
      </Box>
    </>
  );
};

export default RoleCreatePage;

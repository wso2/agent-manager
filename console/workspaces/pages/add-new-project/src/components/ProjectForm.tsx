/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  CircularProgress,
  Form,
  Grid,
  IconButton,
  InputAdornment,
  Typography,
  Stack,
} from "@wso2/oxygen-ui";
import { useEffect, useMemo, useCallback, useState } from "react";
import { useParams } from "react-router-dom";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { debounce } from "lodash";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { AddProjectFormValues, PROJECT_NAME_MAX_LENGTH } from "../form/schema";
import { INPUT_LIMITS } from "@agent-management-platform/types";
import { TextInput } from "@agent-management-platform/views";

interface ProjectFormProps {
  formData: AddProjectFormValues;
  setFormData: React.Dispatch<React.SetStateAction<AddProjectFormValues>>;
  errors: Partial<Record<keyof AddProjectFormValues, string>>;
  validateField: (
    field: keyof AddProjectFormValues,
    value: string,
  ) => string | undefined;
  setFieldError: (
    field: keyof AddProjectFormValues,
    error: string | undefined,
  ) => void;
  checkDirty: (data: AddProjectFormValues) => void;
}

export const ProjectForm = ({
  formData,
  setFormData,
  errors,
  validateField,
  setFieldError,
  checkDirty,
}: ProjectFormProps) => {
  const { orgId } = useParams<{ orgId: string }>();
  const [isNameEditable, setIsNameEditable] = useState(false);

  const handleFieldChange = useCallback(
    (field: keyof AddProjectFormValues, value: string) => {
      const newData = { ...formData, [field]: value };
      setFormData(newData);
      checkDirty(newData);

      const error = validateField(field, value);
      setFieldError(field, error);
    },
    [formData, setFormData, checkDirty, validateField, setFieldError],
  );

  const { mutate: generateName, isPending: isGeneratingName } =
    useGenerateResourceName({
      orgName: orgId,
    });

  // Create debounced function for name generation
  const debouncedGenerateName = useMemo(
    () =>
      debounce((name: string) => {
        if (name.length < 3) {
          handleFieldChange("name", "");
          return;
        }
        generateName(
          {
            displayName: name,
            resourceType: "project",
          },
          {
            onSuccess: (data) => {
              setFormData((prevData) => {
                const newData = { ...prevData, name: data.name };
                checkDirty(newData);
                return newData;
              });
            },
            onError: (error) => {
              // eslint-disable-next-line no-console
              console.error("Failed to generate name:", error);
            },
          },
        );
      }, 500), // 500ms delay
    [generateName, setFormData, checkDirty],
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      debouncedGenerateName.cancel();
    };
  }, [debouncedGenerateName]);

  // Auto-generate name from display name using API with debounce
  useEffect(() => {
    if (isNameEditable) {
      debouncedGenerateName.cancel();
      return;
    }
    if (formData.displayName && formData.displayName.length >= 3) {
      debouncedGenerateName(formData.displayName);
    } else {
      // Clear the name field if display name is empty or too short
      debouncedGenerateName.cancel();
      setFormData((prev) => ({ ...prev, name: "" }));
    }
  }, [
    formData.displayName,
    isNameEditable,
    setFormData,
    debouncedGenerateName,
  ]);

  return (
    <Form.Stack spacing={3}>

      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 6, xl: 4 }}>
          <TextInput
            label="Name"
            id="displayName"
            maxLength={INPUT_LIMITS.NAME}
            hideCharacterCount={formData.displayName.length < INPUT_LIMITS.NAME}
            value={formData.displayName}
            onChange={(e) => handleFieldChange("displayName", e.target.value)}
            placeholder="e.g. Customer Support Platform"
            error={!!errors.displayName}
            helperText={
              isGeneratingName ? (
                <Stack component="span" direction="row" alignItems="center" gap={1}>
                  <CircularProgress size={12} />
                  <Typography variant="caption">Validating name...</Typography>
                </Stack>
              ) : (
                errors.displayName
              )
            }
          />
        </Grid>
        <Grid size={{ xs: 12, md: 6, xl: 4 }}>
          <TextInput
            label="Project ID"
            id="name"
            maxLength={PROJECT_NAME_MAX_LENGTH}
            hideCharacterCount={!isNameEditable || formData.name.length < PROJECT_NAME_MAX_LENGTH}
            value={formData.name}
            onChange={(e) => handleFieldChange("name", e.target.value)}
            placeholder="e.g., customer-support-platform"
            error={!!errors.name}
            helperText={errors.name}
            disabled={!isNameEditable && !formData.name}
            slotProps={{
              htmlInput: { readOnly: !isNameEditable },
              input: {
                endAdornment: !isNameEditable && (
                  <InputAdornment position="end">
                    <IconButton
                      size="small"
                      aria-label="Edit project ID"
                      onClick={() => setIsNameEditable(true)}
                    >
                      <Edit size={16} />
                    </IconButton>
                  </InputAdornment>
                ),
              },
            }}
          />
        </Grid>
        <Grid size={{ xs: 12, md: 12, xl: 8 }}>
          <TextInput
            label="Description (optional)"
            maxLength={INPUT_LIMITS.DESCRIPTION}
            hideCharacterCount={(formData.description?.length ?? 0) < INPUT_LIMITS.DESCRIPTION}
            id="description"
            value={formData.description || ""}
            onChange={(e) => handleFieldChange("description", e.target.value)}
            placeholder="What is this project for?"
            error={!!errors.description}
            helperText={errors.description}
          />
        </Grid>
      </Grid>
    </Form.Stack>
  );
};

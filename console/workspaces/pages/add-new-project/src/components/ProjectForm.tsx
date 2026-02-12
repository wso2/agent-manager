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

import { Box, Form, Select, MenuItem, TextField } from "@wso2/oxygen-ui";
import { useEffect, useMemo, useCallback } from "react";
import { useParams } from "react-router-dom";
import { debounce } from "lodash";
import { useGenerateResourceName } from "@agent-management-platform/api-client";
import { AddProjectFormValues } from "../form/schema";

interface ProjectFormProps {
  formData: AddProjectFormValues;
  setFormData: React.Dispatch<React.SetStateAction<AddProjectFormValues>>;
  errors: Partial<Record<keyof AddProjectFormValues, string>>;
  validateField: (field: keyof AddProjectFormValues, value: string) => string | undefined;
  setFieldError: (field: keyof AddProjectFormValues, error: string | undefined) => void;
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

  const handleFieldChange = useCallback((field: keyof AddProjectFormValues, value: string) => {
    setFormData(prevData => {
      const newData = { ...prevData, [field]: value };
      checkDirty(newData);
      
      // Validate and update error
      const error = validateField(field, value);
      setFieldError(field, error);
      
      return newData;
    });
  }, [setFormData, checkDirty, validateField, setFieldError]);

  const { mutate: generateName } = useGenerateResourceName({
    orgName: orgId,
  });

  // Create debounced function for name generation
  const debouncedGenerateName = useMemo(
    () =>
      debounce((name: string) => {
        generateName({
          displayName: name,
          resourceType: 'project',
        }, {
          onSuccess: (data) => {
            setFormData(prevData => {
              const newData = { ...prevData, name: data.name };
              checkDirty(newData);
              return newData;
            });
          },
          onError: (error) => {
            // eslint-disable-next-line no-console
            console.error('Failed to generate name:', error);
          }
        });
      }, 500), // 500ms delay
    [generateName, setFormData, checkDirty]
  );

  // Cleanup debounce on unmount
  useEffect(() => {
    return () => {
      debouncedGenerateName.cancel();
    };
  }, [debouncedGenerateName]);

  // Auto-generate name from display name using API with debounce
  useEffect(() => {
    if (formData.displayName && formData.displayName.length >= 3) {
      debouncedGenerateName(formData.displayName);
    } else {
      // Clear the name field if display name is empty or too short
      debouncedGenerateName.cancel();
      setFormData(prev => ({ ...prev, name: "" }));
    }
  }, [formData.displayName, setFormData, debouncedGenerateName]);

  return (
    <Form.Stack spacing={3}>
      <Form.Section>
        <Form.Subheader>Project Details</Form.Subheader>
        <Form.Stack spacing={2}>
          <Form.ElementWrapper label="Name" name="displayName">
            <TextField
              id="displayName"
              value={formData.displayName}
              onChange={(e) => handleFieldChange('displayName', e.target.value)}
              placeholder="e.g., Customer Support Platform"
              error={!!errors.displayName}
              helperText={errors.displayName}
              fullWidth
            />
          </Form.ElementWrapper>

          <Form.ElementWrapper label="Description (optional)" name="description">
            <TextField
              id="description"
              value={formData.description}
              onChange={(e) => handleFieldChange('description', e.target.value)}
              placeholder="Short description of this project"
              multiline
              minRows={2}
              maxRows={6}
              error={!!errors.description}
              helperText={errors.description}
              fullWidth
            />
          </Form.ElementWrapper>

          <Box display="none">
            <Form.ElementWrapper label="Deployment Pipeline" name="deploymentPipeline">
              <Select
                id="deploymentPipeline"
                value={formData.deploymentPipeline}
                onChange={(e) => handleFieldChange('deploymentPipeline', e.target.value)}
                error={!!errors.deploymentPipeline}
                fullWidth
              >
                <MenuItem value="default">default</MenuItem>
              </Select>
            </Form.ElementWrapper>
          </Box>
        </Form.Stack>
      </Form.Section>
    </Form.Stack>
  );
};

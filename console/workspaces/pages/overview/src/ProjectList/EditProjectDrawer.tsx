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

import { Box, Button, Card, CardContent, Typography, TextField, Form } from "@wso2/oxygen-ui";
import { Edit } from "@wso2/oxygen-ui-icons-react";
import { 
  DrawerWrapper, 
  DrawerHeader, 
  DrawerContent,
  useFormValidation,
  useDirtyState
} from "@agent-management-platform/views";
import { z } from "zod";
import { useUpdateProject } from "@agent-management-platform/api-client";
import { ProjectResponse, UpdateProjectRequest } from "@agent-management-platform/types";
import { useEffect, useState, useCallback, useMemo } from "react";

interface EditProjectDrawerProps {
  open: boolean;
  onClose: () => void;
  project: ProjectResponse;
  orgId: string;
}

interface EditProjectFormValues {
  name: string;
  displayName: string;
  description?: string;
  deploymentPipeline: string;
}

const editProjectSchema = z.object({
  displayName: z
    .string()
    .trim()
    .min(3, 'Display name must be at least 3 characters')
    .max(100, 'Display name must be at most 100 characters'),
  name: z
    .string()
    .trim()
    .min(3, 'Name must be at least 3 characters')
    .regex(/^[a-z0-9-]+$/, 'Name must be lowercase letters, numbers, and hyphens only (no spaces)')
    .max(50, 'Name must be at most 50 characters'),
  description: z.string().trim().optional(),
  deploymentPipeline: z
    .string()
    .trim()
    .min(1, 'Deployment pipeline is required'),
});

export function EditProjectDrawer({ open, onClose, project, orgId }: EditProjectDrawerProps) {
  const [formData, setFormData] = useState<EditProjectFormValues>({
    name: project.name,
    displayName: project.displayName,
    description: project.description || '',
    deploymentPipeline: project.deploymentPipeline || 'default',
  });

  const { 
    errors, 
    validateField,
    validateForm, 
    clearErrors,
    setFieldError,
  } = useFormValidation<EditProjectFormValues>(editProjectSchema);

  const { checkDirty, resetDirty } = useDirtyState(formData);

  const handleFieldChange = useCallback((field: keyof EditProjectFormValues, value: string) => {
    setFormData(prevData => {
      const newData = { ...prevData, [field]: value };
      checkDirty(newData);
      
      // Validate and update error
      const error = validateField(field, value);
      setFieldError(field, error);
      
      return newData;
    });
  }, [checkDirty, validateField, setFieldError]);

  const { mutate: updateProject, isPending } = useUpdateProject({
    orgName: orgId,
    projName: project.name,
  });

  // Reset form when project changes or drawer opens
  useEffect(() => {
    if (open) {
      setFormData({
        name: project.name,
        displayName: project.displayName,
        description: project.description || '',
        deploymentPipeline: project.deploymentPipeline || 'default',
      });
      clearErrors();
      resetDirty();
    }
  }, [project, open, clearErrors, resetDirty]);

  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault();
    
    if (!validateForm(formData)) {
      return;
    }

    const payload: UpdateProjectRequest = {
      name: formData.name,
      displayName: formData.displayName,
      description: formData.description,
      deploymentPipeline: formData.deploymentPipeline,
    };

    updateProject(payload, {
      onSuccess: () => {
        clearErrors();
        resetDirty();
        onClose();
      },
    });
  }, [formData, validateForm, updateProject, onClose, clearErrors, resetDirty]);

  const isValid = useMemo(() => {
    return (
      formData.displayName.trim().length >= 3 && 
      formData.name.trim().length >= 3 &&
      Object.keys(errors).length === 0
    );
  }, [formData.displayName, formData.name, errors]);

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader
        icon={<Edit size={24} />}
        title="Edit Project"
        onClose={onClose}
      />
      <DrawerContent>
        <form onSubmit={handleSubmit}>
          <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
            <Card variant="outlined">
              <CardContent sx={{ gap: 1, display: "flex", flexDirection: "column" }}>
                <Box display="flex" flexDirection="column" gap={1}>
                  <Typography variant="h5">Project Details</Typography>
                </Box>
                <Box display="flex" flexDirection="column" gap={1}>
                  <Form.ElementWrapper label="Name" name="displayName">
                    <TextField
                      id="displayName"
                      placeholder="e.g., Customer Support Platform"
                      size="small"
                      fullWidth
                      disabled={isPending}
                      value={formData.displayName}
                      onChange={(e) => handleFieldChange('displayName', e.target.value)}
                      error={!!errors.displayName}
                      helperText={errors.displayName}
                    />
                  </Form.ElementWrapper>
                  <Form.ElementWrapper label="Description (optional)" name="description">
                    <TextField
                      id="description"
                      placeholder="Short description of this project"
                      fullWidth
                      size="small"
                      multiline
                      minRows={2}
                      maxRows={6}
                      disabled={isPending}
                      value={formData.description}
                      onChange={(e) => handleFieldChange('description', e.target.value)}
                      error={!!errors.description}
                      helperText={errors.description}
                    />
                  </Form.ElementWrapper>
                </Box>
              </CardContent>
            </Card>
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
                disabled={!isValid || isPending}
              >
                {isPending ? "Updating..." : "Update Project"}
              </Button>
            </Box>
          </Box>
        </form>
      </DrawerContent>
    </DrawerWrapper>
  );
}

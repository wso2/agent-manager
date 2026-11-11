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

import { Box, Button, Form, Grid, Skeleton } from "@wso2/oxygen-ui";
import {
  PageLayout,
  TextInput,
  useDirtyState,
  useFormValidation,
  LoadErrorNotice,
} from "@agent-management-platform/views";
import { z } from "zod";
import { generatePath, Navigate, useNavigate, useParams } from "react-router-dom";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { useGetProject, useUpdateProject } from "@agent-management-platform/api-client";
import { absoluteRouteMap, ProjectResponse, UpdateProjectRequest, INPUT_LIMITS } from "@agent-management-platform/types";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import { useState, useCallback, useMemo } from "react";

interface EditProjectFormProps {
  project: ProjectResponse;
  orgId: string;
  projectHref: string;
}

interface EditProjectFormValues {
  name: string;
  displayName: string;
  description?: string;
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
});

function EditProjectForm({ project, orgId, projectHref }: EditProjectFormProps) {
  const navigate = useNavigate();
  const [formData, setFormData] = useState<EditProjectFormValues>({
    name: project.name,
    displayName: project.displayName,
    description: project.description || '',
  });

  const {
    errors,
    validateField,
    validateForm,
    clearErrors,
    setFieldError,
  } = useFormValidation<EditProjectFormValues>(editProjectSchema);

  const { isDirty: formChanged, checkDirty, resetDirty } = useDirtyState(formData);
  const { allowNavigation } = useUnsavedChangesGuard(formChanged);

  const handleFieldChange = useCallback((field: keyof EditProjectFormValues, value: string) => {
    const newData = { ...formData, [field]: value };
    setFormData(newData);
    checkDirty(newData);
    setFieldError(field, validateField(field, value));
  }, [formData, checkDirty, validateField, setFieldError]);

  const { mutate: updateProject, isPending } = useUpdateProject({
    orgName: orgId,
    projName: project.name,
  });

  const handleCancel = useCallback(() => {
    allowNavigation(() => navigate(projectHref));
  }, [allowNavigation, navigate, projectHref]);

  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault();

    if (!validateForm(formData)) {
      return;
    }

    const payload: UpdateProjectRequest = {
      name: formData.name,
      displayName: formData.displayName,
      description: formData.description,
    };

    updateProject(payload, {
      onSuccess: () => {
        clearErrors();
        resetDirty();
        allowNavigation(() => navigate(projectHref));
      },
    });
  }, [
    formData, validateForm, updateProject, clearErrors, resetDirty,
    allowNavigation, navigate, projectHref,
  ]);

  const isValid = useMemo(() => {
    return (
      formData.displayName.trim().length >= 3 &&
      formData.name.trim().length >= 3 &&
      Object.keys(errors).length === 0
    );
  }, [formData.displayName, formData.name, errors]);

  return (
    <form onSubmit={handleSubmit}>
      <Box display="flex" flexDirection="column" gap={2} flexGrow={1}>
        <Form.Stack spacing={3}>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
              <TextInput
                label="Name"
                id="displayName"
                maxLength={INPUT_LIMITS.NAME}
                hideCharacterCount={formData.displayName.length < INPUT_LIMITS.NAME}
                placeholder="e.g., Customer Support Platform"
                disabled={isPending}
                value={formData.displayName}
                onChange={(e) => handleFieldChange('displayName', e.target.value)}
                error={!!errors.displayName}
                helperText={errors.displayName}
              />
            </Grid>
            <Grid size={{ xs: 12, md: 12, xl: 8 }}>
              <TextInput
                label="Description (optional)"
                id="description"
                maxLength={INPUT_LIMITS.DESCRIPTION}
                hideCharacterCount={(formData.description?.length ?? 0) < INPUT_LIMITS.DESCRIPTION}
                placeholder="What is this project for?"
                disabled={isPending}
                value={formData.description || ''}
                onChange={(e) => handleFieldChange('description', e.target.value)}
                error={!!errors.description}
                helperText={errors.description}
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
            disabled={!isValid || isPending}
          >
            {isPending ? "Updating..." : "Update project"}
          </Button>
        </Box>
      </Box>
    </form>
  );
}

export function EditProject() {
  const { orgId = "", projectId = "" } = useParams<{ orgId: string; projectId: string }>();
  const { canWrite } = useResourceAccess("project");
  const { data: project, isLoading, error, refetch } = useGetProject({
    orgName: orgId,
    projName: projectId,
  });
  const projectHref = generatePath(absoluteRouteMap.children.org.children.projects.path, {
    orgId,
    projectId,
  });

  if (!canWrite) {
    return <Navigate to={projectHref} replace />;
  }

  return (
    <PageLayout
      title="Edit Project"
      disableIcon
      backHref={projectHref}
      backLabel="Back to project"
    >
      {isLoading ? (
        <Skeleton variant="rounded" width="100%" height={400} />
      ) : !project ? (
        <LoadErrorNotice
          entityLabel="project"
          error={error}
          onRetry={() => refetch()}
          backHref={generatePath(
            absoluteRouteMap.children.org.children.projectListing.path,
            { orgId },
          )}
          backLabel="Back to projects"
        />
      ) : (
        <EditProjectForm project={project} orgId={orgId} projectHref={projectHref} />
      )}
    </PageLayout>
  );
}

export default EditProject;

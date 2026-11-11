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

import React, { useCallback, useMemo, useState } from 'react';
import { Alert, Box } from '@wso2/oxygen-ui';
import {
  PageLayout,
  useFormValidation,
  useDirtyState,
  useReturnTo,
} from '@agent-management-platform/views';
import { useUnsavedChangesGuard } from '@agent-management-platform/shared-component';
import { generatePath, Navigate, useNavigate, useParams } from 'react-router-dom';
import { useResourceAccess } from '@agent-management-platform/feature-management';
import { absoluteRouteMap } from '@agent-management-platform/types';
import { addProjectSchema, type AddProjectFormValues } from './form/schema';
import { useCreateProject } from '@agent-management-platform/api-client';
import { CreateButtons } from './components/CreateButtons';
import { ProjectForm } from './components/ProjectForm';

export const AddNewProject: React.FC = () => {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const { canWrite } = useResourceAccess('project');

  const [formData, setFormData] = useState<AddProjectFormValues>({
    name: '',
    displayName: '',
    description: '',
  });

  const {
    errors,
    validateField,
    validateForm,
    clearErrors,
    setFieldError,
  } = useFormValidation<AddProjectFormValues>(addProjectSchema);

  const { isDirty, checkDirty, resetDirty } = useDirtyState(formData);
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  const params = useMemo(() => ({
    orgName: orgId ?? 'default',
  }), [orgId]);

  const { mutate: createProject, isPending, error } = useCreateProject(params);

  const returnTo = useReturnTo(generatePath(
    absoluteRouteMap.children.org.children.projectListing.path,
    { orgId: orgId ?? '' }
  ));

  const handleCancel = useCallback(() => {
    allowNavigation(() => navigate(returnTo));
  }, [allowNavigation, navigate, returnTo]);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] = useState<
    Record<string, string>
  >({});

  const handleCreateProject = useCallback(() => {
    if (!validateForm(formData)) {
      setLastSubmittedValidationErrors(errors);
      return;
    } else {
      setLastSubmittedValidationErrors({});
    }

    createProject({
      name: formData.name,
      displayName: formData.displayName,
      description: formData.description?.trim() || undefined,
    }, {
      onSuccess: () => {
        resetDirty();
        clearErrors();
        allowNavigation(() => navigate(generatePath(
          absoluteRouteMap.children.org.children.projects.path,
          {
            orgId: params.orgName ?? '',
            projectId: formData.name,
          }
        )));
      },
      onError: (e: unknown) => {
        // eslint-disable-next-line no-console
        console.error('Failed to create project:', e);
      }
    });
  }, [formData, validateForm, createProject,
    navigate, allowNavigation, params.orgName,
    resetDirty, clearErrors, errors]);

  if (!canWrite) {
    return (
      <Navigate
        to={generatePath(absoluteRouteMap.children.org.children.projectListing.path, {
          orgId: orgId ?? '',
        })}
        replace
      />
    );
  }

  return (
    <PageLayout
      title="Create a Project"
      backHref={returnTo}
      avatar={null}
      backLabel="Back"
    >
      <Box display="flex" flexDirection="column" gap={2}>
        <ProjectForm
          formData={formData}
          setFormData={setFormData}
          errors={errors}
          validateField={validateField}
          setFieldError={setFieldError}
          checkDirty={checkDirty}
        />
        {!!error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error instanceof Error ? error.message : 'Failed to create project'}
          </Alert>
        )}
        <CreateButtons
          lastSubmittedValidationErrors={lastSubmittedValidationErrors}
          isPending={isPending}
          onCancel={handleCancel}
          onSubmit={handleCreateProject}
          isNameEmpty={!formData.name.trim()}
        />
      </Box>
    </PageLayout>
  );
};

export default AddNewProject;

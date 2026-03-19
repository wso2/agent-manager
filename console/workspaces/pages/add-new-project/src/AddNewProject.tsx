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
import { PageLayout, useFormValidation, useDirtyState } from '@agent-management-platform/views';
import { generatePath, useNavigate, useParams } from 'react-router-dom';
import { absoluteRouteMap } from '@agent-management-platform/types';
import { addProjectSchema, type AddProjectFormValues } from './form/schema';
import { useCreateProject } from '@agent-management-platform/api-client';
import { CreateButtons } from './components/CreateButtons';
import { ProjectForm } from './components/ProjectForm';

export const AddNewProject: React.FC = () => {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();

  const [formData, setFormData] = useState<AddProjectFormValues>({
    name: '',
    displayName: '',
    description: '',
    deploymentPipeline: 'default',
  });

  const {
    errors,
    validateField,
    validateForm,
    clearErrors,
    setFieldError,
  } = useFormValidation<AddProjectFormValues>(addProjectSchema);

  const { checkDirty, resetDirty } = useDirtyState(formData);

  const params = useMemo(() => ({
    orgName: orgId ?? 'default',
  }), [orgId]);

  const { mutate: createProject, isPending, error } = useCreateProject(params);

  const handleCancel = useCallback(() => {
    navigate(generatePath(
      absoluteRouteMap.children.org.path,
      { orgId: orgId ?? '' }
    ));
  }, [navigate, orgId]);

  const [lastSubmittedValidationErrors, setLastSubmittedValidationErrors] = useState<
    typeof errors
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
      deploymentPipeline: formData.deploymentPipeline,
    }, {
      onSuccess: () => {
        resetDirty();
        clearErrors();
        navigate(generatePath(
          absoluteRouteMap.children.org.children.projects.path,
          {
            orgId: params.orgName ?? '',
            projectId: formData.name,
          }
        ));
      },
      onError: (e: unknown) => {
        // eslint-disable-next-line no-console
        console.error('Failed to create project:', e);
      }
    });
  }, [formData, validateForm, createProject,
    navigate, params.orgName, resetDirty, clearErrors, errors]);

  return (
    <PageLayout
      title="Create a New Project"
      description="Create a new project to organize and manage your agents."
      disableIcon
      backHref={generatePath(
        absoluteRouteMap.children.org.path,
        { orgId: orgId ?? '' }
      )}
      backLabel="Back to Organization"
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
          mode="deploy"
        />
      </Box>
    </PageLayout>
  );
};

export default AddNewProject;

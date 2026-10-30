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
import { useUnsavedChangesGuard } from '@agent-management-platform/shared-component';
import { generatePath, useNavigate, useParams } from 'react-router-dom';
import { absoluteRouteMap } from '@agent-management-platform/types';
import { addProjectSchema, type AddProjectFormValues } from './form/schema';
import { useCreateProject, useListDeploymentPipelines } from '@agent-management-platform/api-client';
import { CreateButtons } from './components/CreateButtons';
import { ProjectForm } from './components/ProjectForm';

export const AddNewProject: React.FC = () => {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();

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
  const { data: pipelinesData, isLoading: isPipelinesLoading } =
    useListDeploymentPipelines({ orgName: orgId });

  // Projects start on the "default" pipeline (else the first one); it can be changed later.
  const defaultPipeline = useMemo(() => {
    const pipelines = pipelinesData?.deploymentPipelines ?? [];
    return (pipelines.find((p) => p.name === 'default') ?? pipelines[0])?.name;
  }, [pipelinesData]);

  const handleCancel = useCallback(() => {
    allowNavigation(() => navigate(generatePath(
      absoluteRouteMap.children.org.path,
      { orgId: orgId ?? '' }
    )));
  }, [allowNavigation, navigate, orgId]);

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

    if (!defaultPipeline) {
      setLastSubmittedValidationErrors({
        deploymentPipeline: 'No deployment pipeline is available. Create one in the organization first.',
      });
      return;
    }

    createProject({
      name: formData.name,
      displayName: formData.displayName,
      description: formData.description?.trim() || undefined,
      deploymentPipeline: defaultPipeline,
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
    navigate, allowNavigation, params.orgName, defaultPipeline, resetDirty, clearErrors, errors]);

  return (
    <PageLayout
      title="Create a Project"
      backHref={generatePath(
        absoluteRouteMap.children.org.path,
        { orgId: orgId ?? '' }
      )}
      avatar={null}
      backLabel="Back to Projects"
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
          isPending={isPending || isPipelinesLoading}
          onCancel={handleCancel}
          onSubmit={handleCreateProject}
          isNameEmpty={!formData.name.trim()}
          mode="deploy"
        />
      </Box>
    </PageLayout>
  );
};

export default AddNewProject;

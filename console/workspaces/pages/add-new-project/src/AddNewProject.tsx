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

import React, { useCallback, useMemo } from 'react';
import { Alert, Box } from '@wso2/oxygen-ui';
import { PageLayout } from '@agent-management-platform/views';
import { generatePath, useNavigate, useParams } from 'react-router-dom';
import { absoluteRouteMap } from '@agent-management-platform/types';
import { useForm, FormProvider } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import { addProjectSchema, type AddProjectFormValues } from './form/schema';
import { useCreateProject } from '@agent-management-platform/api-client';
import { CreateButtons } from './components/CreateButtons';
import { ProjectForm } from './components/ProjectForm';

export const AddNewProject: React.FC = () => {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const methods = useForm<AddProjectFormValues>({
    resolver: yupResolver(addProjectSchema),
    defaultValues: {
      name: '',
      displayName: '',
      description: '',
      deploymentPipeline: 'default',
    },
    mode: 'all',
    reValidateMode: 'onChange',
  });

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

  const onSubmit = useCallback((values: AddProjectFormValues) => {
    createProject({
      name: values.name,
      displayName: values.displayName,
      description: values.description?.trim() || undefined,
      deploymentPipeline: values.deploymentPipeline,
    }, {
      onSuccess: () => {
        navigate(generatePath(
          absoluteRouteMap.children.org.children.projects.path,
          {
            orgId: params.orgName ?? '',
            projectId: values.name,
          }
        ));
      },
      onError: (e: unknown) => {
        // Error handling is done by the mutation
        // eslint-disable-next-line no-console
        console.error('Failed to create project:', e);
      }
    });
  }, [createProject, navigate, params.orgName]);

  const handleCreateProject = useMemo(() => methods.handleSubmit(onSubmit), [methods, onSubmit]);

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
        <FormProvider {...methods}>
          <ProjectForm />
        </FormProvider>
        {!!error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error instanceof Error ? error.message : 'Failed to create project'}
          </Alert>
        )}
        <CreateButtons
          isValid={methods.formState.isValid}
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

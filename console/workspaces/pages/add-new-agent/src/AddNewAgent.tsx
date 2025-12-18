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
import { PageLayout } from '@agent-management-platform/views'
import { generatePath, useNavigate, useParams } from 'react-router-dom';
import { absoluteRouteMap, OrgProjPathParams } from '@agent-management-platform/types';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import { addAgentSchema, type AddAgentFormValues } from './form/schema';
import { useCreateAgent, useListAgents } from '@agent-management-platform/api-client';
import { AgentFlowRouter } from './components/AgentFlowRouter';
import { CreateButtons } from './components/CreateButtons';
import { useAgentFlow } from './hooks/useAgentFlow';
import { buildAgentCreationPayload } from './utils/buildAgentPayload';

export const AddNewAgent: React.FC = () => {
  const navigate = useNavigate();
  const { orgId, projectId } = useParams<{ orgId: string; projectId?: string }>();
  const methods = useForm<AddAgentFormValues>({
    resolver: yupResolver(addAgentSchema),
    defaultValues: {
      name: '',
      displayName: '',
      description: '',
      repositoryUrl: '',
      branch: 'main',
      appPath: '',
      runCommand: 'python main.py',
      language: 'python',
      languageVersion: '3.11',
      interfaceType: 'DEFAULT',
      port: '' as unknown as number,
      basePath: '/',
      openApiFileName: '',
      openApiContent: '',
      env: [{ key: '', value: '' }],
      deploymentType: 'new',
    },
    mode: 'all', // Validate on change, blur, and submit
    reValidateMode: 'onChange',
  });
  const { mutate: createAgent, isPending, error } = useCreateAgent();
  const { data: agents} = useListAgents({
    orgName: orgId ?? 'default',
    projName: projectId ?? 'default'
  });
  const params = useMemo<OrgProjPathParams>(() => ({
    orgName: orgId ?? 'default',
    projName: projectId ?? 'default'
  }), [orgId, projectId]);

  const { selectedOption, handleSelect } = useAgentFlow(methods, orgId, projectId);

  const handleCancel = useCallback(() => {
    navigate(generatePath(
      absoluteRouteMap.children.org.children.projects.path,
      { orgId: orgId ?? '', projectId: projectId ?? 'default' }
    ));
  }, [navigate, orgId, projectId]);

  const onSubmit = useCallback((values: AddAgentFormValues) => {
    const payload = buildAgentCreationPayload(values, params);

    createAgent(payload, {
      onSuccess: () => {
        navigate(generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents.path,
          {
            orgId: params.orgName ?? '',
            projectId: params.projName ?? '',
            agentId: payload.body.name
          })
        + "?setup=true"
        );
      },
      onError: (e: unknown) => {
        // TODO: Show error toast/notification to user
        // eslint-disable-next-line no-console
        console.error('Failed to create agent:', e);
      }
    });
  }, [createAgent, navigate, params]);

  const handleAddAgent = useMemo(() => methods.handleSubmit(onSubmit), [methods, onSubmit]);

  const pageMetadata = useMemo(() => {
    if (selectedOption === 'new') {
      return {
        title: 'Create a Platform-Hosted Agent',
        description: 'Specify the source repository, select the agent type, and deploy it on the platform.',
        backable: true
      };
    }
    if (selectedOption === 'existing') {
      return {
        title: 'Register an Externally-Hosted Agent',
        description: 'Provide basic information to register your externally-hosted agent on the platform.',
        backable: true
      };
    }
    return {
      title: 'Add a New Agent',
      description: 'Choose how you want to get started. You can deploy an agent on the platform or register an agent that already runs elsewhere.',
      backable: false
    };
  }, [selectedOption]);

  const { title, description, backable } = pageMetadata;
  const hasAgents = Boolean(agents?.agents?.length && agents?.agents?.length > 0);

  const backHref = useMemo(() => {
    if (!hasAgents) {
      return undefined;
    }

    const route = backable
      ? absoluteRouteMap.children.org.children.projects.children.newAgent.path
      : absoluteRouteMap.children.org.children.projects.path;

    return generatePath(route, {
      orgId: orgId ?? '',
      projectId: projectId ?? 'default'
    });
  }, [backable, hasAgents, orgId, projectId]);

  return (
    <PageLayout title={title} description={description} 
    disableIcon
      backHref={backHref}
      backLabel={backable ? 'Back to Agent Hosting Options' : "Back to Projects Home"}
    >
      <Box display="flex" flexDirection="column" gap={2}>
        <AgentFlowRouter
          methods={methods}
          onSelect={handleSelect}
        />

        {!!error && (
          <Alert severity="error" sx={{ mt: 2 }}>
            {error instanceof Error ? error.message : 'Failed to create agent'}
          </Alert>
        )}

        {selectedOption && (
          <CreateButtons
            isValid={methods.formState.isValid}
            isPending={isPending}
            onCancel={handleCancel}
            onSubmit={handleAddAgent}
            mode={selectedOption === 'existing' ? 'connect' : 'deploy'}
          />
        )}
      </Box>
    </PageLayout>
  );
};

export default AddNewAgent;

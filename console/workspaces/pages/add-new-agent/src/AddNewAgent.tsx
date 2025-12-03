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

import React, { useState } from 'react';
import { Alert, Box } from '@mui/material';
import { MainActionPanel, PageLayoutContent } from '@agent-management-platform/views'
import { AgentSummaryPanel } from './components/AgentSummaryPanel';
import { generatePath, useNavigate, useParams } from 'react-router-dom';
import { absoluteRouteMap, CreateAgentRequest, OrgProjPathParams } from '@agent-management-platform/types';
import { useForm } from 'react-hook-form';
import { yupResolver } from '@hookform/resolvers/yup';
import { addAgentSchema, type AddAgentFormValues } from './form/schema';
import { useCreateAgent } from '@agent-management-platform/api-client';
import { NewAgentOptions } from './components/NewAgentOptions';
import { NewAgentFromSource } from './components/NewAgentFromSource';
import { ConnectNewAgent } from './components/ConnectNewAgent';

export const AddNewAgent: React.FC = () => {
  const navigate = useNavigate();
  const [selectedOption, setSelectedOption] = useState<'new' | 'existing' | null>(null);
  const { orgId, projectId } = useParams<{ orgId: string; projectId?: string }>();
  const methods = useForm<AddAgentFormValues>({
    resolver: yupResolver(addAgentSchema),
    defaultValues: {
      name: '',
      displayName: '',
      description: '',
      repositoryUrl: '',
      branch: 'main',
      appPath: '/',
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

  const handleCancel = () => {
    navigate(generatePath(absoluteRouteMap.children.org.children.projects.path, { orgId: orgId ?? '', projectId: projectId ?? 'default' }));
  };

  const handleAddAgent = methods.handleSubmit((values) => {
    const params = { orgName: orgId ?? 'default', projName: projectId ?? 'default' };

    const getAgentCreationPayload = (data: AddAgentFormValues): 
    { params: OrgProjPathParams; body: CreateAgentRequest } => {
      if (data.deploymentType === 'new') {
        return {
          params,
          body: {
            name: data.name,
            displayName: data.displayName,
            description: data.description?.trim() || undefined,
            provisioning: {
              type: 'internal',
              repository: {
                url: data.repositoryUrl ?? '',
                branch: data.branch ?? 'main',
                appPath: data.appPath ?? '/',
              },
            },
            runtimeConfigs: {
              language: data.language ?? 'python',
              languageVersion: data.languageVersion ?? '3.11',
              runCommand: data.runCommand ?? '',
              env: data.env.filter(e => e.key && e.value).map(
                e => ({ key: e.key!, value: e.value! })),
            },
            inputInterface: {
              type: data.interfaceType,
              ...(data.interfaceType === 'CUSTOM' && {
                customOpenAPISpec: {
                  port: Number(data.port),
                  basePath: data.basePath || '/',
                  schema: { content: data.openApiContent ?? '' },
                },
              }),
            },
          }
        };
      }
      return {
        params,
        body: {
          name: data.name,
          displayName: data.displayName,
          description: data.description,
          provisioning: {
            type: 'external',
          },
        }
      };
    };


    const payload = getAgentCreationPayload(values);
    
    createAgent(payload,
      {
        onSuccess: () => {
          navigate(generatePath(
            absoluteRouteMap.children.org.children.projects.children.agents.path, { orgId: params.orgName ?? '', projectId: params.projName ?? '', agentId: payload.body.name }));
        },
        onError: (e: unknown) => {
          // TODO: Show error toast/notification to user
          // eslint-disable-next-line no-console
          console.error('Failed to create agent:', e);
        }
      }
    );
  });

  const handleSelect = (option: 'new' | 'existing') => {
    setSelectedOption(option);
    methods.setValue('deploymentType', option);
  };

  const createContent = () => {
    if (selectedOption === 'new') {
      return <NewAgentFromSource methods={methods} />;
    }
    if (selectedOption === 'existing') {
      return <ConnectNewAgent methods={methods} />;
    }
    return <NewAgentOptions onSelect={handleSelect} />;
  };

  const getPageMetadata = () => {
    if (selectedOption === 'new') {
      return {
        title: 'Deploy New Agent',
        description: 'Deploy your AI agent to development environment from a GitHub repository'
      };
    }
    if (selectedOption === 'existing') {
      return {
        title: 'Connect Existing Agent',
        description: 'Integrate your already deployed agent with the platform using OpenTelemetry'
      };
    }
    return {
      title: 'Add New Agent',
      description: 'Choose how to add your agent to the platform'
    };
  };

  const { title, description } = getPageMetadata();

  return (
    <PageLayoutContent title={title} description={description}>
      {createContent()}

      {!!error && (
        <Alert severity="error" sx={{ mt: 2 }}>
          {error instanceof Error ? error.message : 'Failed to create agent'}
        </Alert>
      )}

      {selectedOption && (
        <>
          <Box position="relative" height={180} />
          <MainActionPanel>
            <AgentSummaryPanel
              control={methods.control}
              errors={methods.formState.errors}
              isValid={methods.formState.isValid}
              isPending={isPending}
              onCancel={handleCancel}
              onSubmit={handleAddAgent}
              mode={selectedOption === 'existing' ? 'connect' : 'deploy'}
            />
          </MainActionPanel>
        </>
      )}
    </PageLayoutContent>
  );
};

export default AddNewAgent;

import { CreateAgentRequest, OrgProjPathParams } from '@agent-management-platform/types';
import { AddAgentFormValues } from '../form/schema';

export const buildAgentCreationPayload = (
  data: AddAgentFormValues,
  params: OrgProjPathParams
): { params: OrgProjPathParams; body: CreateAgentRequest } => {
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
          env: data.env
            .filter(envVar => envVar.key && envVar.value)
            .map(envVar => ({ key: envVar.key!, value: envVar.value! })),
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


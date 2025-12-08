import { httpGET, httpPOST, SERVICE_BASE } from '../utils';
import {
  DeployAgentPathParams,
  DeployAgentRequest,
  DeploymentListResponse,
  DeploymentResponse,
  ListAgentDeploymentsPathParams,
  GetAgentEndpointsPathParams,
  EndpointsResponse,
  EnvironmentQuery,
  GetAgentConfigurationsPathParams,
  ConfigurationResponse,
  ListEnvironmentsPathParams,
  EnvironmentListResponse,
  GetDeploymentPipelinePathParams,
  DeploymentPipelineResponse,
} from '@agent-management-platform/types';



// eslint-disable-next-line max-len
export async function deployAgent(params: DeployAgentPathParams, body: DeployAgentRequest, getToken?: () => Promise<string>)
: Promise<DeploymentResponse> {
    const { orgName = "default", projName = "default", agentName } = params;
    const token = getToken ? await getToken() : undefined;
    const res = await httpPOST(
        `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/deployments`,
        body,
        { token },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}

// eslint-disable-next-line max-len
export async function listAgentDeployments(params: ListAgentDeploymentsPathParams, getToken?: () => Promise<string>)
: Promise<DeploymentListResponse> {
    const { orgName = "default", projName = "default", agentName } = params;
    const token = getToken ? await getToken() : undefined;
    const res = await httpGET(
        `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/deployments`,
        { token },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}

// eslint-disable-next-line max-len
export async function getAgentEndpoints(params: GetAgentEndpointsPathParams, query: EnvironmentQuery, getToken?: () => Promise<string>)
: Promise<EndpointsResponse> {
    const { orgName = "default", projName = "default", agentName } = params;
    const token = getToken ? await getToken() : undefined;
    const search = { environment: query.environment };
    const res = await httpGET(
        `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/endpoints`,
        { searchParams: search, token },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}

// eslint-disable-next-line max-len
export async function getAgentConfigurations(params: GetAgentConfigurationsPathParams, query: EnvironmentQuery, getToken?: () => Promise<string>)
: Promise<ConfigurationResponse> {
    const { orgName = "default", projName = "default", agentName } = params;
    const token = getToken ? await getToken() : undefined;
    const search = { environment: query.environment };
    const res = await httpGET(
        `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/agents/${encodeURIComponent(agentName)}/configurations`,
        { searchParams: search, token },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}

// eslint-disable-next-line max-len
export async function listEnvironments(params: ListEnvironmentsPathParams, getToken?: () => Promise<string>)
: Promise<EnvironmentListResponse> {
    const { orgName = "default" } = params;
    const token = getToken ? await getToken() : undefined;
    const res = await httpGET(
        `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/environments`,
        { token },
    );
    if (!res.ok) throw await res.json();
    const result = await res.json() as EnvironmentListResponse;
    const singleEnv = result.find(env => env.name === 'development' || env.name === 'default');
    return singleEnv? [singleEnv] : []
}

// eslint-disable-next-line max-len
export async function getDeploymentPipeline(params: GetDeploymentPipelinePathParams, getToken?: () => Promise<string>)
: Promise<DeploymentPipelineResponse> {
    const { orgName = "default", projName = "default" } = params;
    const token = getToken ? await getToken() : undefined;
    const res = await httpGET(
        `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}/deployment-pipeline`,
        { token },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}



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

import { agentGatewayPath, agentProjectQuery, httpGET, httpPUT, SERVICE_BASE } from '../../shared/utils';
import type {
  UpdateAgentDeploySettingsPathParams,
  UpdateAgentDeploySettingsRequest,
  DeploymentListResponse,
  ListAgentDeploymentsPathParams,
  EnvironmentQuery,
  GetAgentConfigurationsPathParams,
  ConfigurationResponse,
  ListEnvironmentsPathParams,
  GetEnvironmentPathParams,
  EnvironmentListResponse,
  Environment,
} from '@agent-management-platform/types';

export async function updateAgentDeploySettings(params: UpdateAgentDeploySettingsPathParams,
     body: UpdateAgentDeploySettingsRequest, getToken?: () => Promise<string>)
: Promise<void> {
    const { projName = "default", agentName } = params;

    if (!agentName) {
        throw new Error("agentName is required");
    }

    const token = getToken ? await getToken() : undefined;
    // The settings apply to the default environment, so the body names none.
    const settings: Partial<UpdateAgentDeploySettingsRequest> = { ...body };
    delete settings.environmentName;
    const res = await httpPUT(
        agentGatewayPath({ agentName }, "/deploy-settings"),
        settings,
        { token, searchParams: agentProjectQuery({ projName }) },
    );
    if (!res.ok) throw await res.json();
}

// eslint-disable-next-line max-len
export async function listAgentDeployments(params: ListAgentDeploymentsPathParams, getToken?: () => Promise<string>)
: Promise<DeploymentListResponse> {
    const { projName = "default", agentName } = params;
    
    if (!agentName) {
        throw new Error("agentName is required");
    }
    
    const token = getToken ? await getToken() : undefined;
    const res = await httpGET(
        agentGatewayPath({ agentName }, "/deployments"),
        { token, searchParams: agentProjectQuery({ projName }) },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}

// eslint-disable-next-line max-len
export async function getAgentConfigurations(params: GetAgentConfigurationsPathParams, query: EnvironmentQuery, getToken?: () => Promise<string>)
: Promise<ConfigurationResponse> {
    const { projName = "default", agentName } = params;
    
    if (!agentName) {
        throw new Error("agentName is required");
    }
    
    const token = getToken ? await getToken() : undefined;
    // The configurations are always the default environment's.
    void query;
    const res = await httpGET(
        agentGatewayPath({ agentName }, "/configurations"),
        { searchParams: agentProjectQuery({ projName }), token },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}

// eslint-disable-next-line max-len
export async function listEnvironments(_params: ListEnvironmentsPathParams, getToken?: () => Promise<string>)
: Promise<EnvironmentListResponse> {
    const token = getToken ? await getToken() : undefined;
    // The organization comes from the token.
    const res = await httpGET(`${SERVICE_BASE}/environments`, { token });
    if (!res.ok) throw await res.json();
    return res.json();

}

// eslint-disable-next-line max-len
export async function getEnvironment(params: GetEnvironmentPathParams, getToken?: () => Promise<string>)
: Promise<Environment> {
    const { envName } = params;

    if (!envName) {
        throw new Error("envName is required");
    }

    const token = getToken ? await getToken() : undefined;
    const res = await httpGET(
        `${SERVICE_BASE}/environments/${encodeURIComponent(envName)}`,
        { token },
    );
    if (!res.ok) throw await res.json();
    return res.json();
}

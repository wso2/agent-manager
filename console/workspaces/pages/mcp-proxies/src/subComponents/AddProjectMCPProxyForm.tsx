/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useCallback, useState } from "react";
import { generatePath, useParams } from "react-router-dom";
import {
  useCreateProjectMCPProxy,
  useListEnvironments,
  usePutProjectMCPProxyEnvironment,
} from "@agent-management-platform/api-client";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { environmentLabel } from "@agent-management-platform/shared-component";
import {
  AddMCPProxyForm,
  type AddMCPProxyFormValues,
} from "./AddMCPProxyForm";
import { type EndpointDraft } from "./EndpointFormFields";
import { draftToEndpoint } from "./mcpEndpoints";
import {
  type BindingFailure,
  type ProjectProxyViewState,
  upstreamAuthRequest,
} from "./projectBinding";

/**
 * Registers a project MCP server by its URL. The proxy fronts the endpoint's URL, then each
 * environment the endpoint is assigned gets its own binding with the endpoint's credential;
 * Agent Manager discovers each binding's tools through its gateway once it is deployed.
 */
export function AddProjectMCPProxyForm({
  projectId,
  onCancel,
}: {
  projectId: string;
  onCancel: () => void;
}) {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const createProjectMCPProxy = useCreateProjectMCPProxy();
  const putProjectBinding = usePutProjectMCPProxyEnvironment();
  const { data: environments = [] } = useListEnvironments({ orgName: orgId });
  const [error, setError] = useState<string>();

  const handleSubmit = useCallback(
    async (values: AddMCPProxyFormValues, endpoints: EndpointDraft[]) => {
      setError(undefined);
      const proxyId = values.id;
      const endpoint = draftToEndpoint(endpoints[0], 0);
      const params = { orgName: orgId, projName: projectId, proxyId };
      try {
        await createProjectMCPProxy.mutateAsync({
          params,
          body: {
            name: values.name,
            version: values.version,
            description: values.description,
            context: values.context,
            upstream: { url: endpoint.upstream?.main?.url ?? "" },
          },
        });
      } catch (err) {
        // Registering only creates: an id the project already has is refused and left as it
        // is. That includes a server a deleted project of the same name left, which no list
        // shows.
        setError(
          (err as { status?: number })?.status === 412
            ? `The id "${proxyId}" is already taken in this project, possibly by an MCP server a deleted project with the same name left. Choose another name.`
            : ((err as { message?: string })?.message ??
                "Failed to register the MCP server."),
        );
        throw err;
      }
      const auth = endpoint.upstream?.main?.auth;
      // The server is registered either way: environments it could not be deployed to are
      // named on its page, where Edit deploys it to them again.
      const bindingFailures: BindingFailure[] = [];
      for (const binding of endpoint.environments) {
        const env = environments.find((e) => e.id === binding.environmentUuid);
        const envName = env?.name;
        if (!envName) {
          bindingFailures.push({
            environment: binding.environmentUuid,
            message: "The environment was not found.",
          });
          continue;
        }
        try {
          await putProjectBinding.mutateAsync({
            params: { ...params, envName },
            body: {
              ...(binding.gatewayId ? { gatewayId: binding.gatewayId } : {}),
              ...(auth?.header ? { upstreamAuth: upstreamAuthRequest(auth) } : {}),
              ...(endpoint.resilience ? { resilience: endpoint.resilience } : {}),
            },
          });
        } catch (err) {
          bindingFailures.push({
            environment: environmentLabel(env),
            message:
              (err as { message?: string })?.message ?? "Deployment failed.",
          });
        }
      }
      const state: ProjectProxyViewState | undefined = bindingFailures.length
        ? { bindingFailures }
        : undefined;
      return {
        to: generatePath(
          absoluteRouteMap.children.org.children.projects.children.mcpProxies
            .children.view.path,
          { orgId, projectId, proxyId },
        ),
        state,
      };
    },
    [createProjectMCPProxy, environments, orgId, projectId, putProjectBinding],
  );

  return (
    <AddMCPProxyForm
      onCancel={onCancel}
      onSubmit={handleSubmit}
      isSubmitting={createProjectMCPProxy.isPending || putProjectBinding.isPending}
      error={error}
      // A project server's context is served under the project.
      defaultContext={(handle) => `/${handle}`}
      handleHelperText="Generated from the name. Unique in the organization and immutable once registered."
      contextHelperText={(context) =>
        `Served under the project, at /${projectId}${context || "/<handle>"}.`
      }
      endpointsTitle="Endpoint"
      endpointsDescription="Add the MCP server's public URL and assign it to the environments to deploy it to. Each environment is secured with an API key."
      emptyStateText="No endpoint added yet."
      maxEndpoints={1}
    />
  );
}

export default AddProjectMCPProxyForm;

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
import { Alert, Skeleton } from "@wso2/oxygen-ui";
import { generatePath, useParams } from "react-router-dom";
import {
  useDeleteProjectMCPProxyEnvironment,
  useGetProjectMCPProxy,
  useListEnvironments,
  usePutProjectMCPProxyEnvironment,
  useUpdateProjectMCPProxy,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type Environment,
  type ProjectMCPProxy,
} from "@agent-management-platform/types";
import { LoadErrorNotice, PageLayout } from "@agent-management-platform/views";
import { EditMCPProxyForm, type EditMCPProxyFormValues } from "./EditMCPProxy";
import { type EndpointDraft } from "./EndpointFormFields";
import { draftToEndpoint } from "./mcpEndpoints";
import {
  planURLProxySave,
  projectProxyEndpointDraft,
  proxyRequestFrom,
  upstreamAuthRequest,
} from "./projectBinding";

function ProjectEditForm({
  proxy,
  orgId,
  projectId,
  environments,
  viewHref,
}: {
  proxy: ProjectMCPProxy;
  orgId: string;
  projectId: string;
  environments: Environment[];
  viewHref: string;
}) {
  const updateProxy = useUpdateProjectMCPProxy();
  const putBinding = usePutProjectMCPProxyEnvironment();
  const removeBinding = useDeleteProjectMCPProxyEnvironment();
  const [error, setError] = useState<unknown>();
  // A server registered by URL is one endpoint deployed to its environments; one published
  // from a component gets its endpoint from the component, so only its details are edited here.
  const isURLProxy = !proxy.source;
  // Taken once, when the page opens: a save that fails part-way refetches the proxy, and a retry
  // must still see the user's edits as edits, not compare them with what was just saved.
  const [initialEndpoints] = useState(() =>
    isURLProxy ? [projectProxyEndpointDraft(proxy)] : [],
  );

  const handleSave = useCallback(
    async (values: EditMCPProxyFormValues, endpoints: EndpointDraft[]) => {
      setError(undefined);
      const params = { orgName: orgId, projName: projectId, proxyId: proxy.id };
      const details = {
        ...proxyRequestFrom(proxy),
        name: values.name.trim(),
        version: values.version.trim(),
        context: values.context?.trim() || proxy.context,
        description: values.description?.trim() || undefined,
      };
      try {
        if (!isURLProxy) {
          await updateProxy.mutateAsync({ params, body: details });
          return;
        }
        const draft = endpoints[0];
        if (!draft) {
          throw new Error("Add the MCP server's endpoint to save it.");
        }
        const endpoint = draftToEndpoint(draft, 0);
        const auth = endpoint.upstream?.main?.auth;
        const bound = new Set(proxy.environments.map((env) => env.environmentUuid));
        // Resolved before anything is saved: an environment that is gone fails the save whole.
        const added = draft.environments
          .filter((uuid) => !bound.has(uuid))
          .map((uuid) => {
            const envName = environments.find((env) => env.id === uuid)?.name;
            if (!envName) {
              throw new Error(
                "An environment selected for the server no longer exists. Reload the page and try again.",
              );
            }
            return { envName, gatewayId: draft.gatewayByEnv?.[uuid] };
          });
        const steps = planURLProxySave(
          proxy,
          initialEndpoints[0] ?? draft,
          draft,
          {
            url: endpoint.upstream?.main?.url ?? "",
            upstreamAuth: upstreamAuthRequest(auth),
            resilience: endpoint.resilience,
          },
          added,
        );
        for (const step of steps) {
          if (step.kind === "remove") {
            await removeBinding.mutateAsync({ ...params, envName: step.envName });
          } else if (step.kind === "proxy") {
            await updateProxy.mutateAsync({
              params,
              body: {
                ...details,
                upstream: { url: step.url, resetBindingAuth: step.resetBindingAuth || undefined },
              },
            });
          } else {
            await putBinding.mutateAsync({
              params: { ...params, envName: step.envName },
              body: step.body,
            });
          }
        }
      } catch (err) {
        setError(err);
        throw err;
      }
    },
    [
      environments,
      initialEndpoints,
      isURLProxy,
      orgId,
      projectId,
      proxy,
      putBinding,
      removeBinding,
      updateProxy,
    ],
  );

  return (
    <EditMCPProxyForm
      proxy={proxy}
      initialEndpoints={initialEndpoints}
      orgId={orgId}
      environments={environments}
      viewHref={viewHref}
      onSave={handleSave}
      isPending={
        updateProxy.isPending || putBinding.isPending || removeBinding.isPending
      }
      error={error}
      contextPlaceholder={`/${proxy.id}`}
      contextHelperText={`Served under the project, at /${projectId}<context>.`}
      maxEndpoints={1}
      endpointsDescription={
        isURLProxy
          ? "Editing the endpoint updates the upstream URL and authentication for every environment it is deployed to. Unassigning an environment removes the server from it."
          : undefined
      }
      endpointsNotice={
        isURLProxy ? undefined : (
          <Alert severity="info">
            This MCP Server is published from the {proxy.source?.componentName}{" "}
            component (endpoint {proxy.source?.endpointName}). Its endpoint and
            environments come from the component; configure each environment
            from the server&apos;s page. Its publisher may set the details above
            again when it next publishes it.
          </Alert>
        )
      }
    />
  );
}

/** Edits a project MCP server with the org MCP server's edit form. */
export function EditProjectMCPProxy() {
  const {
    orgId = "",
    projectId = "",
    proxyId = "",
  } = useParams<{ orgId: string; projectId: string; proxyId: string }>();
  const {
    data: proxy,
    isLoading,
    error,
    refetch,
  } = useGetProjectMCPProxy({ orgName: orgId, projName: projectId, proxyId });
  const { data: environments = [], isLoading: isEnvironmentsLoading } =
    useListEnvironments({
      orgName: orgId,
    });
  const viewHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.mcpProxies.children
      .view.path,
    { orgId, projectId, proxyId },
  );

  return (
    <PageLayout
      title="Edit MCP Server"
      disableIcon
      backHref={viewHref}
      backLabel="Back to MCP server"
    >
      {isLoading || isEnvironmentsLoading ? (
        <Skeleton variant="rounded" width="100%" height={400} />
      ) : !proxy ? (
        <LoadErrorNotice
          entityLabel="MCP server"
          error={error}
          onRetry={() => refetch()}
          backHref={generatePath(
            absoluteRouteMap.children.org.children.projects.children.mcpProxies
              .path,
            { orgId, projectId },
          )}
          backLabel="Back to MCP servers"
        />
      ) : (
        <ProjectEditForm
          proxy={proxy}
          orgId={orgId}
          projectId={projectId}
          environments={environments}
          viewHref={viewHref}
        />
      )}
    </PageLayout>
  );
}

export default EditProjectMCPProxy;

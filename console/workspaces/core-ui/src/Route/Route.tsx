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

import { Suspense } from "react";
import {
  BrowserRouter,
  Routes,
  Route,
  useParams,
  useLocation,
  Outlet,
  Navigate,
  generatePath,
} from "react-router-dom";
import { OxygenLayout } from "../Layouts";
import { Protected } from "../Providers/Protected";
import { ErrorPages } from "@agent-management-platform/shared-component";
import {
  Login,
  LazyOverviewOrg,
  LazyOverviewProject,
  LazyOverviewComponent,
  LazyConfigureComponent,
  LazyAgentIdComponent,
  LazyLLMProvidersOrg,
  LazyMCPProxiesOrg,
  LazyAddLLMProvidersComponent,
  LazyLLMProvidersComponent,
  LazyViewLLMProviderComponent,
  LazyAddLLMProvidersOrg,
  LazyViewMCPServerComponent,
  LazyGatewaysOrg,
  LazyThunderInstancesOrg,
  LazySettingsOrg,
  LazyDeploymentPipelinesOrg,
  LazyEnvironmentsOrg,
  LazyCatalogOrg,
  LazyAddNewAgent,
  LazyAddNewProject,
  LazyBuildComponent,
  LazySecurityComponent,
  LazyDeploymentComponent,
  LazyPublishOrg,
  LazyTestComponent,
  LazyTracesComponent,
  LazyLogsComponent,
  LazyMetricsComponent,
  LazyEvalEvaluatorsOrg,
  LazyCreateEvaluatorOrg,
  LazyViewEvaluatorOrg,
  LazyEditEvaluatorOrg,
  LazyEvalMonitorsComponent,
  LazyCreateMonitorComponent,
  LazyViewMonitorComponent,
  LazyEditMonitorComponent,
  LazyCompareMonitorComponent,
} from "../pages";
import { LoadingFallback } from "../components/LoadingFallback";
import {
  relativeRouteMap,
  absoluteRouteMap,
} from "@agent-management-platform/types";
import {
  useExternalPageModules,
  type ExternalPageModule,
} from "@agent-management-platform/views";
import {
  useListOrganizations,
  useGetProject,
  useGetAgent,
  usePageViewTracking,
} from "@agent-management-platform/api-client";
import { MountPoints } from "../types";

// Back-compat redirect: the Identities pages (users/roles/groups) moved from
// /org/:orgId/identities/* to /org/:orgId/settings/identities/*. Preserve any
// trailing sub-path so existing deep links keep working.
function LegacyIdentitiesRedirect() {
  const { orgId, "*": rest } = useParams<{ orgId: string; "*": string }>();
  const base = generatePath(
    absoluteRouteMap.children.org.children.settings.children.identities.path,
    { orgId },
  );
  return <Navigate to={rest ? `${base}/${rest}` : base} replace />;
}

// Remounts the Security page on agent change so per-agent component state
// (Create-key dialog open flag, newly-issued-key banner) does not leak
// between agents when navigating via the sidebar.
function SecurityRouteElement() {
  const { agentId } = useParams();
  return <LazySecurityComponent key={agentId} />;
}
// Reports one page view per navigation.
//
// Mounted once here, inside BrowserRouter and beside Routes, rather than per
// page: a single caller means no page can be forgotten and none can
// double-report. It renders nothing and is inert unless console analytics is
// enabled — see usePageViewTracking.
function PageViewTracker() {
  const { pathname } = useLocation();
  usePageViewTracking(pathname);
  return null;
}

function GuardedOutlet({
  isLoading,
  isError,
  title,
  message,
}: {
  isLoading: boolean;
  isError: boolean;
  title: string;
  message: string;
}) {
  if (isLoading) {
    return <LoadingFallback />;
  }
  if (isError) {
    return <ErrorPages.CustomError title={title} message={message} />;
  }
  // Suspense boundary for the lazy route components rendered below this
  // guard, so every lazy import gets a loading fallback.
  return (
    <Suspense fallback={<LoadingFallback />}>
      <Outlet />
    </Suspense>
  );
}

function OrgGuard() {
  const { orgId } = useParams();
  const { data, isLoading, error } = useListOrganizations();
  const orgExists = data?.organizations?.some((o) => o.name === orgId);
  // A failed list query is a load failure (network/server/auth), not a missing
  // org — only treat the org as "not found" when the list loaded successfully
  // but does not contain it.
  const loadFailed = !!error;
  return (
    <GuardedOutlet
      isLoading={isLoading}
      isError={loadFailed || (!isLoading && !orgExists)}
      title={
        loadFailed ? "Failed to Load Organization" : "Organization Not Found"
      }
      message={
        loadFailed
          ? "Something went wrong while loading your organizations. Please try again."
          : `The organization "${orgId}" doesn't exist or you don't have access to it.`
      }
    />
  );
}

function ProjectGuard() {
  const { orgId, projectId } = useParams();
  const { isLoading, isError } = useGetProject({
    orgName: orgId,
    projName: projectId,
  });
  return (
    <GuardedOutlet
      isLoading={isLoading}
      isError={isError}
      title="Project Not Found"
      message={`The project "${projectId}" doesn't exist or you don't have access to it.`}
    />
  );
}

function AgentGuard() {
  const { orgId, projectId, agentId } = useParams();
  const { isLoading, isError } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  return (
    <GuardedOutlet
      isLoading={isLoading}
      isError={isError}
      title="Agent Not Found"
      message={`The agent "${agentId}" doesn't exist or you don't have access to it.`}
    />
  );
}

export function RootRouter() {
  const externalOrgPageModules = useExternalPageModules();

  const { projectPageModules, orgPageModules, componentPageModules } =
    externalOrgPageModules.reduce(
      (acc, module) => {
        if (module.mountPoint === MountPoints.ProjectLevelPage) {
          acc.projectPageModules.push(module);
        } else if (module.mountPoint === MountPoints.OrgLevelPage) {
          acc.orgPageModules.push(module);
        } else if (module.mountPoint === MountPoints.ComponentLevelPage) {
          acc.componentPageModules.push(module);
        }
        return acc;
      },
      {
        projectPageModules: [] as ExternalPageModule[],
        orgPageModules: [] as ExternalPageModule[],
        componentPageModules: [] as ExternalPageModule[],
      },
    );

  // Monitor pages live under environment/:envId/evaluation/monitor; build the
  // shared prefix once instead of repeating the deep route chain per route.
  const monitorRoutes =
    relativeRouteMap.children.org.children.projects.children.agents.children
      .environment.children.evaluation.children.monitor;
  const monitorBase =
    relativeRouteMap.children.org.children.projects.children.agents.children
      .environment.children.evaluation.path +
    "/" +
    monitorRoutes.path;

  return (
    <BrowserRouter>
      <PageViewTracker />
      <Routes>
        <Route
          path={relativeRouteMap.children.login.path}
          element={<Login />}
        />
        <Route
          path={"/"}
          element={
            <Protected>
              <OxygenLayout />
            </Protected>
          }
        >
          <Route
            path={relativeRouteMap.children.org.path}
            element={<OrgGuard />}
          >
            <Route index element={<LazyOverviewOrg />} />
            {orgPageModules.map((module) => (
              <Route
                key={module.path}
                path={module.path + "/*"}
                element={
                  <Suspense fallback={<LoadingFallback />}>
                    <module.pageComponent />
                  </Suspense>
                }
              />
            ))}
            <Route
              path={
                relativeRouteMap.children.org.children.thunderInstances.path +
                "/*"
              }
              element={
                <Suspense fallback={<LoadingFallback />}>
                  <LazyThunderInstancesOrg />
                </Suspense>
              }
            />
            <Route
              path={relativeRouteMap.children.org.children.gateways.path + "/*"}
              element={<LazyGatewaysOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.settings.path + "/*"
              }
              element={<LazySettingsOrg />}
            />
            {/* Back-compat: identities pages moved under Settings → IDP Settings */}
            <Route path="identities/*" element={<LegacyIdentitiesRedirect />} />
            <Route
              path={
                relativeRouteMap.children.org.children.deploymentPipelines
                  .path + "/*"
              }
              element={<LazyDeploymentPipelinesOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.environments.path + "/*"
              }
              element={<LazyEnvironmentsOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.llmProviders.path + "/*"
              }
              element={<LazyLLMProvidersOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.mcpProxies.path + "/*"
              }
              element={<LazyMCPProxiesOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.llmProviders.path +
                "/" +
                relativeRouteMap.children.org.children.llmProviders.children.add
                  .path
              }
              element={
                <Suspense fallback={<LoadingFallback />}>
                  <LazyAddLLMProvidersOrg />
                </Suspense>
              }
            />
            <Route
              path={relativeRouteMap.children.org.children.evaluators.path}
              element={<LazyEvalEvaluatorsOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.evaluators.path +
                "/" +
                relativeRouteMap.children.org.children.evaluators.children
                  .create.path
              }
              element={<LazyCreateEvaluatorOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.evaluators.path +
                "/" +
                relativeRouteMap.children.org.children.evaluators.children.edit
                  .path
              }
              element={<LazyEditEvaluatorOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.evaluators.path +
                "/" +
                relativeRouteMap.children.org.children.evaluators.children.view
                  .path
              }
              element={<LazyViewEvaluatorOrg />}
            />
            <Route
              path={relativeRouteMap.children.org.children.catalog.path + "/*"}
              element={<LazyCatalogOrg />}
            />
            <Route
              path={relativeRouteMap.children.org.children.newProject.path}
              element={
                <Suspense fallback={<LoadingFallback />}>
                  <LazyAddNewProject />
                </Suspense>
              }
            />
            <Route
              path={relativeRouteMap.children.org.children.projects.path}
              element={<ProjectGuard />}
            >
              <Route index element={<LazyOverviewProject />} />
              {projectPageModules.map((module) => (
                <Route
                  key={module.path}
                  path={module.path + "/*"}
                  element={
                    <Suspense fallback={<LoadingFallback />}>
                      <module.pageComponent />
                    </Suspense>
                  }
                />
              ))}
              <Route
                path={
                  relativeRouteMap.children.org.children.projects.children
                    .newAgent.path + "/*"
                }
                element={
                  <Suspense fallback={<LoadingFallback />}>
                    <LazyAddNewAgent />
                  </Suspense>
                }
              />
              <Route
                path={
                  relativeRouteMap.children.org.children.projects.children
                    .agents.path
                }
                element={<AgentGuard />}
              >
                <Route index element={<LazyOverviewComponent />} />
                {componentPageModules.map((module) => (
                  <Route
                    key={module.path}
                    path={module.path + "/*"}
                    element={
                      <Suspense fallback={<LoadingFallback />}>
                        <module.pageComponent />
                      </Suspense>
                    }
                  />
                ))}
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.path
                  }
                  element={
                    <Suspense fallback={<LoadingFallback />}>
                      <LazyConfigureComponent />
                    </Suspense>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.agentId.path
                  }
                  element={
                    <Suspense fallback={<LoadingFallback />}>
                      <LazyAgentIdComponent />
                    </Suspense>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.llmProviders.path
                  }
                  element={<LazyLLMProvidersComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.llmProviders.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.llmProviders.children
                      .add.path
                  }
                  element={
                    <Suspense fallback={<LoadingFallback />}>
                      <LazyAddLLMProvidersComponent />
                    </Suspense>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.llmProviders.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.llmProviders.children
                      .edit.path
                  }
                  element={
                    <Suspense fallback={<LoadingFallback />}>
                      <LazyAddLLMProvidersComponent />
                    </Suspense>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.llmProviders.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.llmProviders.children
                      .view.path
                  }
                  element={
                    <Suspense fallback={<LoadingFallback />}>
                      <LazyViewLLMProviderComponent />
                    </Suspense>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.mcpProxies.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.configure.children.mcpProxies.children
                      .view.path
                  }
                  element={
                    <Suspense fallback={<LoadingFallback />}>
                      <LazyViewMCPServerComponent />
                    </Suspense>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.build.path
                  }
                  element={<LazyBuildComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.deployment.path
                  }
                  element={<LazyDeploymentComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.environment.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.security.path
                  }
                  element={<SecurityRouteElement />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.publish.path + "/*"
                  }
                  element={<LazyPublishOrg />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.environment.path
                  }
                >
                  <Route
                    path={
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.tryOut.path + "/*"
                    }
                    element={<LazyTestComponent />}
                  />
                  <Route
                    path={
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.observability
                        .path +
                      "/" +
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.observability
                        .children.traces.path
                    }
                    element={<LazyTracesComponent />}
                  />
                  <Route
                    path={
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.observability
                        .path +
                      "/" +
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.observability
                        .children.logs.path
                    }
                    element={<LazyLogsComponent />}
                  />
                  <Route
                    path={
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.observability
                        .path +
                      "/" +
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.observability
                        .children.metrics.path
                    }
                    element={<LazyMetricsComponent />}
                  />
                  <Route
                    path={monitorBase}
                    element={<LazyEvalMonitorsComponent />}
                  />
                  <Route
                    path={
                      monitorBase + "/" + monitorRoutes.children.create.path
                    }
                    element={<LazyCreateMonitorComponent />}
                  />
                  <Route
                    path={monitorBase + "/" + monitorRoutes.children.edit.path}
                    element={<LazyEditMonitorComponent />}
                  />
                  <Route
                    path={
                      monitorBase + "/" + monitorRoutes.children.compare.path
                    }
                    element={<LazyCompareMonitorComponent />}
                  />
                  <Route
                    path={
                      monitorBase +
                      "/" +
                      monitorRoutes.children.view.path +
                      "/*"
                    }
                    element={<LazyViewMonitorComponent />}
                  />
                  <Route path="*" element={<ErrorPages.NotFound />} />
                </Route>

                <Route path="*" element={<ErrorPages.NotFound />} />
              </Route>

              <Route path="*" element={<ErrorPages.NotFound />} />
            </Route>
            <Route path="*" element={<ErrorPages.NotFound />} />
          </Route>
          <Route path="*" element={<ErrorPages.NotFound />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

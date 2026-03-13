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
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { OxygenLayout } from "../Layouts";
import { Protected } from "../Providers/Protected";
import {
  Login,
  LazyOverviewOrg,
  LazyOverviewProject,
  LazyOverviewComponent,
  LazyConfigureComponent,
  LazyLLMProvidersOrg,
  LazyAddLLMProvidersComponent,
  LazyLLMProvidersComponent,  LazyViewLLMProviderComponent,  LazyAddLLMProvidersOrg,
  LazyGatewaysOrg,
  LazyAddNewAgent,
  LazyAddNewProject,
  LazyBuildComponent,
  LazyDeploymentComponent,
  LazyTestComponent,
  LazyTracesComponent,
  LazyLogsComponent,
  LazyMetricsComponent,
  LazyEvalEvaluatorsComponent,
  LazyCreateEvaluatorComponent,
  LazyViewEvaluatorComponent,
  LazyEditEvaluatorComponent,
  LazyEvalMonitorsComponent,
  LazyCreateMonitorComponent,
  LazyViewMonitorComponent,
  LazyEditMonitorComponent,
} from "../pages";
import { LoadingFallback } from "../components/LoadingFallback";
import { relativeRouteMap } from "@agent-management-platform/types";
export function RootRouter() {
  return (
    <BrowserRouter>
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
          <Route path={relativeRouteMap.children.org.path}>
            <Route index element={<LazyOverviewOrg />} />
            <Route
              path={
                relativeRouteMap.children.org.children.gateways.path + "/*"
              }
              element={<LazyGatewaysOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.llmProviders.path + "/*"
              }
              element={<LazyLLMProvidersOrg />}
            />
            <Route
              path={
                relativeRouteMap.children.org.children.llmProviders.path + "/" +
                relativeRouteMap.children.org.children.llmProviders.children.add.path
              }
              element={
                <Suspense fallback={<LoadingFallback />}>
                  <LazyAddLLMProvidersOrg />
                </Suspense>
              }
            />
            <Route
              path={relativeRouteMap.children.org.children.newProject.path}
              element={
                <Suspense fallback={<LoadingFallback />}>
                  <LazyAddNewProject />
                </Suspense>
              }
            />
            <Route path={relativeRouteMap.children.org.children.projects.path}>
              <Route index element={<LazyOverviewProject />} />
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
                    .evaluators.path
                }
                element={<LazyEvalEvaluatorsComponent />}
              />
              <Route
                path={
                  relativeRouteMap.children.org.children.projects.children
                    .evaluators.path +
                  "/" +
                  relativeRouteMap.children.org.children.projects.children
                    .evaluators.children.create.path
                }
                element={<LazyCreateEvaluatorComponent />}
              />
              <Route
                path={
                  relativeRouteMap.children.org.children.projects.children
                    .evaluators.path +
                  "/" +
                  relativeRouteMap.children.org.children.projects.children
                    .evaluators.children.edit.path
                }
                element={<LazyEditEvaluatorComponent />}
              />
              <Route
                path={
                  relativeRouteMap.children.org.children.projects.children
                    .evaluators.path +
                  "/" +
                  relativeRouteMap.children.org.children.projects.children
                    .evaluators.children.view.path
                }
                element={<LazyViewEvaluatorComponent />}
              />
              <Route
                path={
                  relativeRouteMap.children.org.children.projects.children
                    .agents.path
                }
              >
                <Route
                  index
                  element={<LazyOverviewComponent />}
                />
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
                      .agents.children.llmProviders.path
                  }
                  element={<LazyLLMProvidersComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.llmProviders.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.llmProviders.children.add.path
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
                      .agents.children.llmProviders.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.llmProviders.children.edit.path
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
                      .agents.children.llmProviders.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.llmProviders.children.view.path
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
                      .agents.children.evaluation.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.evaluators.path
                  }
                  element={<LazyEvalEvaluatorsComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.evaluators.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.evaluators.children.create.path
                  }
                  element={<LazyCreateEvaluatorComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.evaluators.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.evaluators.children.edit.path
                  }
                  element={<LazyEditEvaluatorComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.evaluators.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.evaluators.children.view.path
                  }
                  element={<LazyViewEvaluatorComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.monitor.path
                  }
                  element={<LazyEvalMonitorsComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.monitor.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.monitor.children.create.path
                  }
                  element={<LazyCreateMonitorComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.monitor.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.monitor.children.edit.path
                  }
                  element={<LazyEditMonitorComponent />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.monitor.path +
                    "/" +
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.evaluation
                      .children.monitor.children.view.path +
                    "/*"
                  }
                  element={<LazyViewMonitorComponent />}
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
                        .agents.children.environment.children.observability.path +
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
                        .agents.children.environment.children.observability.path +
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
                        .agents.children.environment.children.observability.path +
                      "/" +
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.observability
                        .children.metrics.path
                    }
                    element={<LazyMetricsComponent />}
                  />

                </Route>
              </Route>
            </Route>
            <Route path="*" element={<div>404 Not Found</div>} />
          </Route>
          <Route path="*" element={<div>404 Not Found</div>} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

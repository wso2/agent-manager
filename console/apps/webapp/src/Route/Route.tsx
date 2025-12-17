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
import { Layout } from "../Layouts";
import { Protected } from "../Providers/Protected";
import {
  Login,
  LazyOverviewOrg,
  LazyOverviewProject,
  LazyOverviewComponent,
  LazyAddNewAgent,
  LazyAddNewProject,
  LazyBuildComponent,
  LazyDeploymentComponent,
  LazyTestComponent,
  LazyTracesComponent,
} from "../pages";
import { LoadingFallback } from "../components/LoadingFallback";
import { relativeRouteMap } from "@agent-management-platform/types";
import {
  AgentInfoPageLayout,
  AgentLayout,
} from "@agent-management-platform/shared-component";
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
              <Layout />
            </Protected>
          }
        >
          <Route path={relativeRouteMap.children.org.path}>
            <Route index element={<LazyOverviewOrg />} />
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
                    .agents.path
                }
                element={<AgentLayout />}
              >
                <Route
                  index
                  element={
                    <AgentInfoPageLayout>
                      <LazyOverviewComponent />
                    </AgentInfoPageLayout>
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
                      .agents.children.observe.path + "/*"
                  }
                  element={<LazyTracesComponent />}
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
                        .path + "/*"
                    }
                    element={<LazyTracesComponent />}
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

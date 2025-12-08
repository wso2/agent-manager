import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Layout } from "../Layouts";
import { Protected } from "../Providers/Protected";
import {
  addNewAgentPageMetaData,
  overviewMetadata,
  testMetadata,
  tracesMetadata,
  buildMetadata,
  Login,
  deploymentMetadata,
} from "../pages";
import { relativeRouteMap } from "@agent-management-platform/types";
import {
  AgentInfoPageLayout,
  AgentLayout,
} from "@agent-management-platform/shared-component";
import { PageLayout } from "@agent-management-platform/views";
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
            <Route index element={<overviewMetadata.levels.organization />} />
            <Route path={relativeRouteMap.children.org.children.projects.path}>
              <Route index element={<overviewMetadata.levels.project />} />
              <Route
                path={
                  relativeRouteMap.children.org.children.projects.children
                    .newAgent.path + "/*"
                }
                element={<addNewAgentPageMetaData.component />}
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
                      <overviewMetadata.levels.component />
                    </AgentInfoPageLayout>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.build.path
                  }
                  element={
                    <PageLayout title={buildMetadata.title} disableIcon>
                      <buildMetadata.levels.component />
                    </PageLayout>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.deployment.path
                  }
                  element={
                    <PageLayout title={deploymentMetadata.title} disableIcon>
                      <deploymentMetadata.levels.component />
                    </PageLayout>
                  }
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.observe.path + "/*"
                  }
                  element={<tracesMetadata.levels.component />}
                />
                <Route
                  path={
                    relativeRouteMap.children.org.children.projects.children
                      .agents.children.environment.path
                  }
                  // element={<EnvSubNavBar />}
                >
                  <Route
                    path={
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.tryOut.path
                    }
                    element={
                      <PageLayout title={testMetadata.title} disableIcon>
                        <testMetadata.levels.component />
                      </PageLayout>
                    }
                  />
                  <Route
                    path={
                      relativeRouteMap.children.org.children.projects.children
                        .agents.children.environment.children.observability
                        .path + "/*"
                    }
                    element={<tracesMetadata.levels.component />}
                  />
                </Route>
              </Route>
            </Route>
            <Route path="*" element={<div>404 Not Found</div>} />
          </Route>
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

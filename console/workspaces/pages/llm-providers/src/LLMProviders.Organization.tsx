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

import React from "react";
import { PageLayout } from "@agent-management-platform/views";
import { LLMProviderTable } from "./subComponents/LLMProviderTable";
import {
  generatePath,
  Navigate,
  Route,
  Routes,
  useParams,
} from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { DeployLLMProviderPage } from "./subComponents/DeployLLMProviderPage";
import { ViewLLMProvider } from "./subComponents/ViewLLMProvider";

export const LLMProvidersOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  return (
    <Routes>
      <Route
        index
        element={
          <PageLayout title="LLM Service Providers" disableIcon>
            <LLMProviderTable />
          </PageLayout>
        }
      />
      <Route path="view/:providerId" element={<ViewLLMProvider />} />
      <Route
        path="view/:providerId/deploy"
        element={<DeployLLMProviderPage />}
      />
      <Route
        path="*"
        element={
          <Navigate
            to={generatePath(
              absoluteRouteMap.children.org.children.llmProviders.path,
              { orgId },
            )}
            replace
          />
        }
      />
    </Routes>
  );
};

export default LLMProvidersOrganization;

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
import { Button } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { PageLayout } from "@agent-management-platform/views";
import { useListLLMProviders } from "@agent-management-platform/api-client";
import { LLMProviderTable } from "./subComponents/LLMProviderTable";
import {
  generatePath,
  Link,
  Navigate,
  Route,
  Routes,
  useParams,
} from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { ViewLLMProvider } from "./subComponents/ViewLLMProvider";
import { EditLLMProvider } from "./subComponents/EditLLMProvider";

export const LLMProvidersOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();

  // Same query as the table (deduped); the header CTA only shows once the list has items.
  const { data: providersList } = useListLLMProviders({ orgName: orgId });
  const hasItems = (providersList?.providers?.length ?? 0) > 0;

  // One definition, used both as the page action and as the empty state's CTA.
  const createButton = (
    <Button
      variant="contained"
      color="primary"
      component={Link}
      to={generatePath(
        absoluteRouteMap.children.org.children.llmProviders.children.add.path,
        { orgId },
      )}
      startIcon={<Plus size={16} />}
    >
      Add Service Provider
    </Button>
  );

  return (
    <Routes>
      <Route
        index
        element={
          <PageLayout
            title="LLM Service Providers"
            disableIcon
            actions={hasItems ? createButton : undefined}
          >
            <LLMProviderTable createAction={createButton} />
          </PageLayout>
        }
      />
      <Route path="view/:providerId" element={<ViewLLMProvider />} />
      <Route path="view/:providerId/edit" element={<EditLLMProvider />} />
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

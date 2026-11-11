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
import { generatePath, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap, type AgentKindResponse } from "@agent-management-platform/types";
import { CatalogKindListing } from "./subComponents/CatalogKindListing";
import { useListAgentKinds } from "@agent-management-platform/api-client";

export const CatalogList: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const { data, isLoading } = useListAgentKinds({ orgName: orgId ?? "" });

  const getViewPath = (item: AgentKindResponse) =>
    generatePath(absoluteRouteMap.children.org.children.catalog.children.kindDetails.path, {
      orgId: orgId ?? "",
      kindId: item.name,
    });

  return (
    <PageLayout
      title="Agent Catalog"
      disableIcon
    >
      <CatalogKindListing
        items={data?.kinds ?? []}
        isLoading={isLoading}
        getViewPath={getViewPath}
      />
    </PageLayout>
  );
};

export default CatalogList;

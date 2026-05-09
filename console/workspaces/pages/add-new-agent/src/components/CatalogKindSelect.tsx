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
import { generatePath, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { CatalogKindListing, DUMMY_CATALOG_LIST, type CatalogItem } from "@agent-management-platform/agent-kind";

export const CatalogKindSelect: React.FC = () => {
  const { orgId, projectId } = useParams<{ orgId: string; projectId: string }>();

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.projects.children.newAgent.children.create.path,
    { orgId: orgId ?? "", projectId: projectId ?? "default" },
  );

  const getViewPath = (item: CatalogItem) =>
    generatePath(
      absoluteRouteMap.children.org.children.projects.children.newAgent.children.create.children.catalog.children.withKind.path,
      { orgId: orgId ?? "", projectId: projectId ?? "default", kindId: item.id },
    );

  return (
    <PageLayout
      title="Select an Agent Kind"
      description="Browse the catalog and pick a kind to create your agent from."
      disableIcon
      backHref={backHref}
      backLabel="Back to Source Type Selection"
    >
      <CatalogKindListing items={DUMMY_CATALOG_LIST} getViewPath={getViewPath} />
    </PageLayout>
  );
};

export default CatalogKindSelect;

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
import { Navigate, Route, Routes, generatePath, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { CatalogKindDetails } from "./Catalog.KindDetails";
import { CatalogList } from "./Catalog.List";

export const CatalogOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();

  return (
    <Routes>
      <Route index element={<CatalogList />} />
      <Route path="kind/:kindId" element={<CatalogKindDetails />} />
      <Route
        path="*"
        element={
          <Navigate
            to={generatePath(
              absoluteRouteMap.children.org.children.catalog.path,
              { orgId },
            )}
          />
        }
      />
    </Routes>
  );
};

export default CatalogOrganization;

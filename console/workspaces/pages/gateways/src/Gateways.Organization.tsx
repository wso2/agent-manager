/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useState } from "react";
import { PageLayout } from "@agent-management-platform/views";
import {
  generatePath,
  Navigate,
  Route,
  Routes,
  useParams,
} from "react-router-dom";
import {
  absoluteRouteMap,
  type GatewayResponse,
} from "@agent-management-platform/types";
import { AIGatewaysTable } from "./subComponents/AIGatewaysTable";
import { AddAIGatewayOrganization } from "./AddAIGateway.Organization";
import { ViewGateway } from "./subComponents/ViewGateway";
import { EditGatewayDrawer } from "./subComponents/EditGatewayDrawer";

export const GatewaysOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const [editDrawerOpen, setEditDrawerOpen] = useState(false);
  const [gatewayToEdit, setGatewayToEdit] = useState<GatewayResponse | null>(
    null,
  );

  const handleEditGateway = (gateway: GatewayResponse) => {
    setGatewayToEdit(gateway);
    setEditDrawerOpen(true);
  };

  const handleEditDrawerClose = () => {
    setEditDrawerOpen(false);
    setGatewayToEdit(null);
  };

  return (
    <>
      <Routes>
        <Route
          index
          element={
            <PageLayout title="AI Gateways" disableIcon>
              <AIGatewaysTable onEditGateway={handleEditGateway} />
            </PageLayout>
          }
        />
        <Route path="add" element={<AddAIGatewayOrganization />} />
        <Route path="view/:gatewayId" element={<ViewGateway />} />
        <Route
          path="*"
          element={
            <Navigate
              to={generatePath(
                absoluteRouteMap.children.org.children.gateways.path,
                { orgId },
              )}
            />
          }
        />
      </Routes>

      {gatewayToEdit && (
        <EditGatewayDrawer
          open={editDrawerOpen}
          onClose={handleEditDrawerClose}
          gateway={gatewayToEdit}
          orgId={orgId ?? ""}
          onSuccess={handleEditDrawerClose}
        />
      )}
    </>
  );
};

export default GatewaysOrganization;

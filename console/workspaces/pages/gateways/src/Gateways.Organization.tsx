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
  useNavigate,
  useParams,
} from "react-router-dom";
import {
  absoluteRouteMap,
  type GatewayResponse,
} from "@agent-management-platform/types";
import { AIGatewaysTable } from "./subComponents/AIGatewaysTable";
import { ViewGateway } from "./subComponents/ViewGateway";
import { EditGateway } from "./subComponents/EditGateway";
import { AddSelfHostedGatewayDrawer } from "./subComponents/AddSelfHostedGatewayDrawer";

export const GatewaysOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const [addDrawerOpen, setAddDrawerOpen] = useState(false);

  const handleEditGateway = (gateway: GatewayResponse) => {
    navigate(
      generatePath(absoluteRouteMap.children.org.children.gateways.children.edit.path, {
        orgId: orgId ?? "",
        gatewayId: gateway.uuid,
      }),
    );
  };

  return (
    <>
      <Routes>
        <Route
          index
          element={
            <PageLayout title="Gateways" disableIcon>
              <AIGatewaysTable
                onEditGateway={handleEditGateway}
                onAddGateway={() => setAddDrawerOpen(true)}
              />
            </PageLayout>
          }
        />
        <Route path="view/:gatewayId" element={<ViewGateway />} />
        <Route path="edit/:gatewayId" element={<EditGateway />} />
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

      <AddSelfHostedGatewayDrawer
        open={addDrawerOpen}
        onClose={() => setAddDrawerOpen(false)}
        orgId={orgId ?? ""}
      />
    </>
  );
};

export default GatewaysOrganization;

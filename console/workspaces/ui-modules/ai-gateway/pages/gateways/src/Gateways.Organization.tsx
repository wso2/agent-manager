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

import React from "react";
import { PageLayout, returnToState } from "@agent-management-platform/views";
import { Button } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { useListGateways } from "@agent-management-platform/api-client";
import { useFeatureFlag, useGatewayFeatures } from "@agent-management-platform/feature-management";
import {
  generatePath,
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
  useParams,
} from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { AIGatewaysTable } from "./subComponents/AIGatewaysTable";
import { ViewGateway } from "./subComponents/ViewGateway";
import { EditGateway } from "./subComponents/EditGateway";
import { AddGateway } from "./subComponents/AddGateway";
import { SetupGateway } from "./subComponents/SetupGateway";

export const GatewaysOrganization: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const { canWrite } = useGatewayFeatures();
  const selfHostedEnabled = useFeatureFlag("enableSelfHostedGateways");
  const canAdd = selfHostedEnabled && canWrite;
  const openAddGateway = () =>
    navigate(
      generatePath(absoluteRouteMap.children.org.children.gateways.children.add.path, {
        orgId,
      }),
      { state: returnToState(location) },
    );
  // Shares the table's query; an empty list offers Add Gateway in its empty state instead.
  const { data: gatewaysData } = useListGateways({ orgName: orgId });
  const hasGateways = (gatewaysData?.gateways?.length ?? 0) > 0;
  const addGatewayButton = canAdd && hasGateways ? (
    <Button variant="contained" startIcon={<Plus size={16} />} onClick={openAddGateway}>
      Add gateway
    </Button>
  ) : undefined;

  return (
    <Routes>
      <Route
        index
        element={
          <PageLayout title="Gateways" disableIcon actions={addGatewayButton}>
            <AIGatewaysTable onAddGateway={canAdd ? openAddGateway : undefined} />
          </PageLayout>
        }
      />
      {canAdd && <Route path="add" element={<AddGateway />} />}
      {canWrite && <Route path="setup/:gatewayId" element={<SetupGateway />} />}
      <Route path="view/:gatewayId" element={<ViewGateway />} />
      {canWrite && <Route path="edit/:gatewayId" element={<EditGateway />} />}
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
  );
};

export default GatewaysOrganization;

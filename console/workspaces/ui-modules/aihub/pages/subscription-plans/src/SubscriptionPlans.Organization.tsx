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

import { Button } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, Navigate, Route, Routes, useParams } from "react-router-dom";
import { PageLayout } from "@agent-management-platform/views";
import { useListSubscriptionPlans } from "@agent-management-platform/api-client";
import { useResourceAccess } from "@agent-management-platform/feature-management";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { SubscriptionPlanTable } from "./subComponents/SubscriptionPlanTable";
import { SubscriptionPlanFormPage } from "./subComponents/SubscriptionPlanFormPage";

const plansRoute = absoluteRouteMap.children.org.children.subscriptionPlans;

/** Organization › Manage › Subscription plans: the list, and the create and edit pages. */
export const SubscriptionPlansOrganization = () => {
  const { orgId = "" } = useParams<{ orgId: string }>();
  const { canWrite } = useResourceAccess("subscription_plan");
  // The table's first page (deduped): the header's create button only shows once the org has plans.
  const { data } = useListSubscriptionPlans({ orgName: orgId }, { limit: 10, offset: 0 });
  const hasPlans = (data?.pagination.total ?? 0) > 0;

  const createButton = (
    <Button
      component={Link}
      startIcon={<Plus size={16} />}
      to={generatePath(plansRoute.children.add.path, { orgId })}
      variant="contained"
    >
      Create plan
    </Button>
  );

  return (
    <Routes>
      <Route
        index
        element={
          <PageLayout
            actions={hasPlans && canWrite ? createButton : undefined}
            description="Define the plans your MCP servers offer, and the limits each one gives an agent that subscribes."
            disableIcon
            title="Subscription Plans"
          >
            <SubscriptionPlanTable createAction={canWrite ? createButton : undefined} />
          </PageLayout>
        }
      />
      {canWrite && <Route element={<SubscriptionPlanFormPage />} path="add" />}
      {canWrite && <Route element={<SubscriptionPlanFormPage />} path="edit/:planId" />}
      <Route
        element={<Navigate replace to={generatePath(plansRoute.path, { orgId })} />}
        path="*"
      />
    </Routes>
  );
};

export default SubscriptionPlansOrganization;

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

import { ReceiptText } from "@wso2/oxygen-ui-icons-react";
import type { PageMetadata } from "@agent-management-platform/types";
import { SubscriptionPlansOrganization } from "./SubscriptionPlans.Organization";

export const metaData: PageMetadata = {
  title: "Subscription Plans",
  description: "Subscription plans that MCP servers offer and agents subscribe to",
  icon: ReceiptText,
  path: "/subscription-plans",
  component: SubscriptionPlansOrganization,
  levels: {
    organization: SubscriptionPlansOrganization,
  },
};

export { SubscriptionPlansOrganization };
export { PlanConditionChips } from "./subComponents/PlanConditionChips";
export default SubscriptionPlansOrganization;

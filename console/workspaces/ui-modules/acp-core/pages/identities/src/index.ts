/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Users } from "@wso2/oxygen-ui-icons-react";
import type { PageMetadata } from "@agent-management-platform/types";
import { IdentitiesOrganization } from "./Identities.Organization";

export const metaData: PageMetadata = {
  title: "Identities",
  description: "Manage users, roles, and groups",
  icon: Users,
  path: "/identities",
  component: IdentitiesOrganization,
  levels: {
    organization: IdentitiesOrganization,
  },
};

export const identitiesMetadata = {
  title: metaData.title,
  icon: metaData.icon,
};

export { IdentitiesOrganization };
export default IdentitiesOrganization;

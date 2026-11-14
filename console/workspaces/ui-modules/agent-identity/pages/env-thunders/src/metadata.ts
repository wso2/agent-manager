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

// Nav-only metadata (title + icon). Deliberately has no import of
// ThunderInstancesOrganization or its subcomponents, so consumers that only
// need the sidebar icon/label (e.g. navigationItems.tsx) don't pull the page's
// component tree into their bundle. Route.tsx gets the component separately
// via a dynamic import() of the package root, which keeps it in its own chunk.
import { Folder, IdCard, Shield, Bot } from "@wso2/oxygen-ui-icons-react";

export const thunderInstancesMetadata = {
  title: "Agent ID",
  icon: IdCard,
  children: {
    agents: { title: "Agents", icon: Bot },
    groups: { title: "Groups", icon: Folder },
    roles: { title: "Roles", icon: Shield },
  },
};

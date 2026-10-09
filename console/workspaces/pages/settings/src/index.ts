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

import { Settings } from "@wso2/oxygen-ui-icons-react";
import type { PageMetadata } from "@agent-management-platform/types";
import { SettingsOrganization } from "./Settings.Organization";

export const metaData: PageMetadata = {
  title: "Settings",
  description: "Manage IDP settings and appearance",
  icon: Settings,
  path: "/settings",
  component: SettingsOrganization,
  levels: {
    organization: SettingsOrganization,
  },
};

export const settingsMetadata = {
  title: metaData.title,
  icon: metaData.icon,
};

export { SettingsOrganization };
export { ProjectWebhooks } from "./ProjectWebhooks";
export { AgentWebhooks } from "./AgentWebhooks";
export default SettingsOrganization;

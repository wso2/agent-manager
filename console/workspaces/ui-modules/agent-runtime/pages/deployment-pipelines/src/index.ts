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

import { Server, ServerCrash } from "@wso2/oxygen-ui-icons-react";
import type { PageMetadata } from "@agent-management-platform/types";
import { DeploymentPipelinesOrganization } from "./DeploymentPipelines.Organization";
import { EnvironmentsOrganization } from "./Environments.Organization";

export const metaData: PageMetadata = {
  title: "Deployment Pipelines",
  description: "A page component for Deployment Pipeline and Environment management",
  icon: ServerCrash,
  path: "/deployment-pipelines",
  component: DeploymentPipelinesOrganization,
  levels: {
    organization: DeploymentPipelinesOrganization,
  },
};

export const environmentsMetaData: PageMetadata = {
  title: "Environments",
  description: "A page component for Environment management",
  icon: Server,
  path: "/environments",
  component: EnvironmentsOrganization,
  levels: {
    organization: EnvironmentsOrganization,
  },
};

export { DeploymentPipelinesOrganization, EnvironmentsOrganization };

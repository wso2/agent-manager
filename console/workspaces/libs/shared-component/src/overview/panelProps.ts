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

import type { DeploymentStatus } from "../components/EnvironmentCard/EnvironmentCard";

/**
 * Props for panels at PLUGIN_MOUNT_POINTS.agentOverviewInfo and
 * agentOverviewEnvironment. Modules contribute overview sections for the data
 * they own; acp-core's agent overview renders them with these props.
 */
export interface AgentOverviewPanelProps {
  orgId: string;
  projectId: string;
  agentId: string;
  /** The environment shown (the selected tab). */
  envId: string;
  /** The agent runs outside the platform (externally hosted). */
  external?: boolean;
  /** The agent's deployment status in `envId` (platform-hosted agents). */
  deploymentStatus?: DeploymentStatus;
}

/** Props for panels at PLUGIN_MOUNT_POINTS.projectOverviewSide. */
export interface ProjectOverviewPanelProps {
  orgId: string;
  projectId: string;
}

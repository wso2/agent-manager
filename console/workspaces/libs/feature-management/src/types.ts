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

/**
 * Switches that turn a console feature off. Every key defaults to `false`, so all
 * features are on unless configured otherwise. Augment this interface to add keys.
 */
export interface FeatureEnableConfig {
  /** Environments are managed outside the console; hides the Environments pages. */
  externally_managed_env: boolean;
  /** Gateways are managed outside the console; hides the Gateways pages. */
  externally_managed_gateway: boolean;
  /** Deployment pipelines are managed outside the console; hides their pages. */
  externally_managed_pipeline: boolean;
}

/**
 * What the current user may do. Every key defaults to `true`. A resource has a
 * `<resource>_read` and a `<resource>_write` key (write = create, update, delete).
 * Augment this interface to add keys.
 */
export interface FeatureAccessConfig {
  project_read: boolean;
  project_write: boolean;
  agent_read: boolean;
  agent_write: boolean;
  agent_kind_read: boolean;
  agent_kind_write: boolean;
  agent_identity_read: boolean;
  agent_identity_write: boolean;
  env_read: boolean;
  env_write: boolean;
  gateway_read: boolean;
  gateway_write: boolean;
  pipeline_read: boolean;
  pipeline_write: boolean;
  git_secret_read: boolean;
  git_secret_write: boolean;
  llm_provider_read: boolean;
  llm_provider_write: boolean;
  llm_provider_template_read: boolean;
  llm_provider_template_write: boolean;
  llm_proxy_read: boolean;
  llm_proxy_write: boolean;
  mcp_server_read: boolean;
  mcp_server_write: boolean;
  evaluator_read: boolean;
  evaluator_write: boolean;
  monitor_read: boolean;
  monitor_write: boolean;
  member_read: boolean;
  member_write: boolean;
  role_read: boolean;
  role_write: boolean;
  group_read: boolean;
  group_write: boolean;
  /** Deploy, promote or change an agent's deployment in non-production environments. */
  agent_deploy: boolean;
  /** Also needed, beside `agent_deploy`, to act on production environments. */
  agent_deploy_production: boolean;
  /** Suspend or resume an agent's deployment. */
  agent_suspend: boolean;
}

export type FeatureEnableKey = keyof FeatureEnableConfig;
export type FeatureAccessKey = keyof FeatureAccessConfig;

/** Resources with a `<resource>_read` / `<resource>_write` access pair. */
export type AccessResource = {
  [K in FeatureAccessKey]: K extends `${infer R}_read`
    ? `${R}_write` extends FeatureAccessKey ? R : never
    : never;
}[FeatureAccessKey];

export interface FeatureManagement {
  featureEnable: Partial<FeatureEnableConfig>;
  featureAccess: Partial<FeatureAccessConfig>;
  /** True while the user's access is still being worked out. */
  isAccessLoading: boolean;
  /** Whether the feature is turned off; `false` when unset. */
  isDisabled: (key: FeatureEnableKey) => boolean;
  /** Whether the user has this access; `true` when unset. */
  hasAccess: (key: FeatureAccessKey) => boolean;
}

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

// Query param read by the agent-level "Agent ID" page
// (@agent-management-platform/env-thunders) to pre-select an environment,
// and written by the deep link an EnvironmentCard's "Agent ID" button builds
// (@agent-management-platform/overview). Shared here — both packages already
// depend on @agent-management-platform/types for absoluteRouteMap — so a
// rename can't desync the two sides.
export const IDENTITY_ENV_PARAM = "identityEnv";

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

// A grantable scope, as surfaced by the environment's agent-identity scope
// aggregate. Keyed by `scope` (there is no separate id). A role's assigned
// scope may not currently be in this aggregate (e.g. its owning proxy is no
// longer deployed to this environment) - in that case it's carried as a
// bare `{ scope }` placeholder.
export type ScopeChoice = { scope: string; description?: string };

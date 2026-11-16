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

// Which tab is active on the Configure Agent page, encoded in the URL (not
// component state) so returning from a config's detail view via the back
// button — or the browser back button — lands on the same tab instead of
// resetting to the first one. Shared by Configure.Component.tsx (reads/writes
// it) and the LLM/MCP detail views (write it into their own backHref).
export const CONFIGURE_TAB_PARAM = "tab";
export const CONFIGURE_TAB_KEYS = ["llm", "tools", "a2a"] as const;
export type ConfigureTabKey = (typeof CONFIGURE_TAB_KEYS)[number];
// Environment selected on the A2A tab, by name.
export const CONFIGURE_ENV_PARAM = "env";

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

// The runtime config is injected by public/config.js before React mounts in the
// real app, and by nothing at all under Vitest. @agent-management-platform/auth
// picks its implementation from globalConfig.disableAuth as it loads, so
// without this, importing any module that transitively reaches auth throws
// before a single test runs.
(
  window as unknown as { __RUNTIME_CONFIG__: Record<string, unknown> }
).__RUNTIME_CONFIG__ ??= {};

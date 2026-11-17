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

// Single-environment mode: whether the console shows deployment pipelines.
//
// Pipelines belong to the agent-runtime UI module, so this is a console
// decision, taken from the module host: without agent-runtime (left out by the
// server UI manifest, or not installed) the pipeline hooks stay idle and pages hide
// pipeline UI. It says nothing about how many environments the service has:
// environments always come from the environments API, by their real names and
// UUIDs, and pickers hide themselves only when that API returns exactly one
// (see onlyEnvironmentName in shared-component). With the service as shipped,
// every organization has one environment, so the two agree.
import { useIsUIModuleInstalled } from "@agent-management-platform/views";

/**
 * True when the agent-runtime module is absent, so the console runs without
 * deployment pipelines; undefined while that is still being detected (treat
 * that as loading).
 */
export function useIsSingleEnvironment(): boolean | undefined {
  const runtimeInstalled = useIsUIModuleInstalled("agent-runtime");
  return runtimeInstalled === undefined ? undefined : !runtimeInstalled;
}

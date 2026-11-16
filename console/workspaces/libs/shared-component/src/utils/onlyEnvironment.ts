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
 * The name of the org's only environment, or undefined when there are none,
 * several, or they have not loaded.
 *
 * The rule for every environment picker: when this returns a name, there is
 * nothing to choose, so the picker hides itself and the form uses this name as
 * its value. Derive the value (`chosen || onlyEnvironmentName(envs)`) rather
 * than copying it into state in an effect, so it is never missing at submit
 * time, never counts as an unsaved edit, and never needs a hidden error.
 */
export function onlyEnvironmentName(
  environments: readonly { name: string }[] | undefined,
): string | undefined {
  return environments?.length === 1 ? environments[0].name : undefined;
}

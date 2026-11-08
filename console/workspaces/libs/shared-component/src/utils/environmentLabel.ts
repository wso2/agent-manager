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

/** The parts of an environment a label is made from. */
export interface LabelledEnvironment {
  id?: string;
  name?: string;
  displayName?: string;
}

/**
 * The text that names an environment: its display name, or its name when the display name is
 * missing or blank, or its ID. A blank display name (an empty string) is not missing as far as `??`
 * is concerned, so `displayName ?? name` left an environment with no label at all.
 */
export function environmentLabel(environment: LabelledEnvironment | undefined): string {
  return (
    environment?.displayName?.trim() ||
    environment?.name?.trim() ||
    environment?.id?.trim() ||
    ''
  );
}

/**
 * Keeps one entry for each environment, the first, in order. An environment is the same by its ID,
 * or by its name when it has no ID, so one the endpoint is bound to twice is listed once.
 */
export function uniqueEnvironments<T extends LabelledEnvironment>(environments: readonly T[]): T[] {
  const seen = new Set<string>();
  const unique: T[] = [];
  for (const environment of environments) {
    const key = environment.id?.trim() || environment.name?.trim() || '';
    if (key !== '' && seen.has(key)) {
      continue;
    }
    seen.add(key);
    unique.push(environment);
  }
  return unique;
}

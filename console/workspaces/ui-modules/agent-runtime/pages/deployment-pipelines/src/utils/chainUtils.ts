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

import type { Environment } from "@agent-management-platform/types";

export interface PromotionPathInput {
    sourceEnvironmentRef: string;
    targetEnvironmentRefs: { name: string }[];
}

/** Converts an ordered chain of env names into promotion path objects for the API. */
export function chainToPromotionPaths(chain: string[]): PromotionPathInput[] {
    if (chain.length === 1 && chain[0]) {
        return [{ sourceEnvironmentRef: chain[0], targetEnvironmentRefs: [] }];
    }
    return chain.slice(0, -1).map((env, i) => ({
        sourceEnvironmentRef: env,
        targetEnvironmentRefs: [{ name: chain[i + 1] }],
    }));
}

/**
 * Returns environments available for a given chain slot.
 * Excludes environments already selected in other slots.
 * Pass "add" as index to get options for a new slot at the end.
 */
export function chainOptionsFor(
    envOptions: Environment[],
    chain: string[],
    index: number | "add",
): Environment[] {
    return envOptions.filter((e) =>
        index === "add"
            ? !chain.includes(e.name)
            : !chain.some((v, i) => i !== index && v === e.name),
    );
}

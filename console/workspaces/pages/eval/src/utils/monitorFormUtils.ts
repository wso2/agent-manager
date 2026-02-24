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
 * Shared helpers for monitor creation/editing forms.
 */

import type { MonitorType } from "@agent-management-platform/types";
import type { CreateMonitorFormValues } from "../form/schema";

/**
 * Generates a slug friendly identifier from a free-form display name.
 */
export function slugifyMonitorName(value: string): string {
    return value
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-+|-+$/g, "")
        .slice(0, 60);
}

/**
 * Decides whether the monitor name should be auto-updated based on the new display name.
 */
export function getAutoNameSuggestion(
    previousName: string | undefined,
    previousDisplayName: string | undefined,
    nextDisplayName: string
): string | undefined {
    const nextSlug = slugifyMonitorName(nextDisplayName ?? "");
    const previousSlug = slugifyMonitorName(previousDisplayName ?? "");

    if (!previousName || previousName === previousSlug) {
        return nextSlug;
    }

    return undefined;
}

/**
 * Returns the default trace window for past monitors (last 24 hours).
 */
export function getDefaultPastRange() {
    const end = new Date();
    const start = new Date(end.getTime() - 24 * 60 * 60 * 1000);
    return { start, end };
}

/**
 * Provides the field patch that should be applied when switching between monitor types.
 */
export function getMonitorTypeFieldPatch(type: MonitorType): Partial<CreateMonitorFormValues> {
    if (type === "future") {
        return {
            traceStart: null,
            traceEnd: null,
            intervalMinutes: 10,
        };
    }

    const { start, end } = getDefaultPastRange();
    return {
        traceStart: start,
        traceEnd: end,
        intervalMinutes: undefined,
    };
}

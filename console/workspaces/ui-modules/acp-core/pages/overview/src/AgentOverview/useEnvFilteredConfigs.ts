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

import { useCallback, useMemo, useState } from "react";
import type { AgentModelConfigListItem } from "@agent-management-platform/types";

/** A config's applicability, once its own detail fetch settles. */
export type ConfigResolution = "applicable" | "inapplicable" | "error";

/**
 * Whether a model/MCP config is actually mapped is only knowable from its own
 * envMappings — which the list endpoint doesn't return, so each card resolves
 * its own applicability (see LLMProviderConfigCard/MCPProxyConfigCard) and
 * reports it back here via `reportResolved`. This hook keeps the first
 * `previewLimit` configs (in list order) that resolved as applicable.
 *
 * A config whose own detail fetch fails still resolves (as "error") rather
 * than being silently treated as inapplicable — it counts toward `isSettled`
 * so it doesn't block its siblings from displaying, but is tracked
 * separately via `hasError` so the group can surface a genuine failure
 * instead of quietly rendering as if nothing applied.
 *
 * Waits for every candidate to resolve (rather than stopping as soon as
 * `previewLimit` are found) so `extraCount` — the applicable configs beyond
 * the preview — is an accurate total, not just "at least previewLimit".
 */
export function useEnvFilteredConfigs(
    candidates: AgentModelConfigListItem[],
    previewLimit: number,
) {
    const [resolved, setResolved] = useState<Record<string, ConfigResolution>>({});

    const reportResolved = useCallback((configId: string, resolution: ConfigResolution) => {
        setResolved((prev) => (
            prev[configId] === resolution ? prev : { ...prev, [configId]: resolution }
        ));
    }, []);

    const applicableConfigs = useMemo(
        () => candidates.filter((c) => resolved[c.uuid] === "applicable"),
        [candidates, resolved],
    );

    const visible = useMemo(
        () => applicableConfigs.slice(0, previewLimit),
        [applicableConfigs, previewLimit],
    );

    const isSettled = candidates.length > 0 && candidates.every((c) => c.uuid in resolved);
    const hasError = candidates.some((c) => resolved[c.uuid] === "error");
    const extraCount = isSettled ? Math.max(0, applicableConfigs.length - previewLimit) : 0;

    return {
        visible, reportResolved, isSettled, extraCount, hasError,
    };
}

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

import { getEntityAvatarColor } from "@agent-management-platform/views";

// Brand-ish colors for the handful of providers/config names users are most
// likely to see here; anything else falls back to a deterministic hash color
// so it's still stable across renders instead of random.
const KNOWN_PROVIDER_COLORS: Record<string, string> = {
    "azure-openai": "#0078D4",
    openai: "#10a37f",
    azure: "#0078D4",
    anthropic: "#B45AF2",
    claude: "#B45AF2",
    google: "#4285F4",
    gemini: "#4285F4",
    vertex: "#4285F4",
    mistral: "#FA520F",
    cohere: "#39594D",
    bedrock: "#FF9900",
    aws: "#FF9900",
    meta: "#0668E1",
    llama: "#0668E1",
};

// Sort candidate keys by descending length so specific/longer keys (e.g. "azure-openai")
// are matched before generic/shorter keys (e.g. "openai" or "azure").
const KNOWN_PROVIDER_KEYS = Object.keys(KNOWN_PROVIDER_COLORS).sort(
    (a, b) => b.length - a.length,
);

/** Picks a stable avatar color for a provider/config name — a curated brand
 * color when recognized, otherwise the platform-wide deterministic fallback. */
export function getProviderAvatarColor(name?: string): string {
    if (!name) return getEntityAvatarColor();
    const key = name.trim().toLowerCase();
    if (Object.prototype.hasOwnProperty.call(KNOWN_PROVIDER_COLORS, key)) {
        return KNOWN_PROVIDER_COLORS[key];
    }
    const known = KNOWN_PROVIDER_KEYS.find((k) => key.includes(k));
    if (known) return KNOWN_PROVIDER_COLORS[known];
    return getEntityAvatarColor(key);
}

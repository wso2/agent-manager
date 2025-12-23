/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

export interface EnvVariable {
    key: string;
    value: string;
}

// Strips surrounding quotes from a value (single or double quotes)
function stripQuotes(value: string): string {
    const trimmed = value.trim();
    if (
        (trimmed.startsWith('"') && trimmed.endsWith('"')) ||
        (trimmed.startsWith("'") && trimmed.endsWith("'"))
    ) {
        return trimmed.slice(1, -1);
    }
    return trimmed;
}

// Parses .env file content into an array of key-value pairs
export function parseEnvContent(content: string): EnvVariable[] {
    const lines = content.split(/\r?\n/);
    const envMap = new Map<string, string>();

    for (const line of lines) {
        const trimmedLine = line.trim();

        // Skip empty lines and comments
        if (!trimmedLine || trimmedLine.startsWith('#')) {
            continue;
        }

        // Find the first '=' to split key and value
        const equalIndex = trimmedLine.indexOf('=');
        if (equalIndex === -1) {
            continue; // Skip lines without '='
        }

        const key = trimmedLine.substring(0, equalIndex).trim();
        const rawValue = trimmedLine.substring(equalIndex + 1);
        const value = stripQuotes(rawValue);

        // Skip entries with empty keys
        if (!key) {
            continue;
        }

        // Use Map to handle duplicates (last value wins)
        envMap.set(key, value);
    }

    // Convert Map to array
    return Array.from(envMap.entries()).map(([key, value]) => ({ key, value }));
}

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

import { formatRelativeTime as oxygenFormatRelativeTime } from "@wso2/oxygen-ui";

export function normalizeVersion(value: string): string;
export function normalizeVersion(value: string | undefined): string | undefined;
export function normalizeVersion(value: string | undefined): string | undefined {
  if (value === undefined) return undefined;
  if (!value) return value;
  return value.toLowerCase().startsWith("v") ? value : `v${value}`;
}

// Lives in `views` so the avatar components there can share it; re-exported
// here for the call sites that already import it from this module.
export {
  getAvatarInitials,
  type AvatarInitialsOptions,
} from "@agent-management-platform/views";

/** Capitalizes the first letter of a status/label string, e.g. "active" -> "Active". */
export function capitalize(value: string): string {
  if (!value) return value;
  return value.charAt(0).toUpperCase() + value.slice(1);
}

/**
 * Relative timestamp in the design system's wording ("Just now", "2 hours
 * ago"), falling back to an absolute date once "ago" stops being useful.
 *
 * Wraps Oxygen's formatter rather than restating it, adding only the guards
 * Oxygen's `Date`-only signature lacks: missing and unparseable values.
 */
export function formatRelativeTime(
  value: string | number | Date | undefined,
  options: { fallback?: string } = {},
): string {
  const { fallback = "-" } = options;
  if (value === undefined || value === null || value === "") return fallback;
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  return oxygenFormatRelativeTime(date);
}

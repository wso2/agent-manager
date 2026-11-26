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

import type { BudgetPeriod } from "@agent-management-platform/types";

export const PERIOD_LABELS: Record<BudgetPeriod, string> = {
  day: "Daily",
  week: "Weekly",
  month: "Monthly",
};

/** A filter on a type with no one value: "All projects", "All LLM providers", "All API keys". */
export function allOfKind(label: string): string {
  return `All ${inSentence(label)}s`;
}

/** A label as it reads mid-sentence: lowercase, except acronyms ("API key", "LLM providers"). */
export function inSentence(label: string): string {
  return label
    .split(" ")
    .map((w) => (/^[A-Z]{2,}s?$/.test(w) ? w : w.toLowerCase()))
    .join(" ");
}

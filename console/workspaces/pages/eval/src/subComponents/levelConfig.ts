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

import { type EvaluationLevel } from "@agent-management-platform/types";

export const LEVEL_CONFIG: Record<
  EvaluationLevel,
  { label: string; color: string; unit: string }
> = {
  trace: { label: "Trace", color: "#7c3aed", unit: "traces" },
  agent: { label: "Agent", color: "#3f8cff", unit: "agent executions" },
  llm: { label: "LLM", color: "#22c55e", unit: "LLM invocations" },
};

export const levelChipSx = (
  cfg: (typeof LEVEL_CONFIG)[EvaluationLevel],
  isDark = false,
) => ({
  backgroundColor: `${cfg.color}${isDark ? "40" : "18"}`,
  color: cfg.color,
  fontWeight: 600,
  fontSize: "0.65rem",
  height: 20,
  "& .MuiChip-label": { px: 0.75 },
});

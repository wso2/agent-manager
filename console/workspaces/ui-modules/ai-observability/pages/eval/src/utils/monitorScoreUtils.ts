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

import type {
  EvaluationLevel,
  EvaluatorScoreSummary,
} from "@agent-management-platform/types";
import type { LevelSummary } from "../subComponents/EvaluationSummaryCard";

/** Extract the numeric mean from an evaluator's aggregations map. */
export function getMean(e: EvaluatorScoreSummary): number | null {
  const v = e.aggregations?.["mean"];
  return typeof v === "number" ? v : null;
}

/** Builds the per-level breakdown used by EvaluationSummaryCard. */
export function computeLevelSummaries(
  evaluators: EvaluatorScoreSummary[],
): LevelSummary[] {
  const levelOrder: EvaluationLevel[] = ["trace", "agent", "llm"];
  const levelsPresent = new Set(evaluators.map((e) => e.level));
  return levelOrder
    .filter((lvl) => levelsPresent.has(lvl))
    .map((lvl) => {
      const group = evaluators.filter((e) => e.level === lvl);
      return {
        level: lvl,
        evaluatorCount: group.length,
        uniqueCount: Math.max(...group.map((e) => e.count), 0),
        totalEvaluations: group.reduce((s, e) => s + e.count, 0),
        skippedCount: group.reduce((s, e) => s + e.skippedCount, 0),
      };
    });
}

/** Averages the mean score across all evaluators that have a numeric mean. */
export function computeAverageScore(
  evaluators: EvaluatorScoreSummary[],
): number | null {
  const means = evaluators.map(getMean).filter((m): m is number => m !== null);
  if (means.length === 0) {
    return null;
  }
  return means.reduce((acc, m) => acc + m, 0) / means.length;
}

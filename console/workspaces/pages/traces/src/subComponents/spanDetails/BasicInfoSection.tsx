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

import {
  Span,
  LLMData,
  AgentData,
  EmbeddingData,
  RetrieverData,
  EvaluatorScoreWithMonitor,
} from "@agent-management-platform/types";
import { Chip, Stack, Tooltip } from "@wso2/oxygen-ui";
import {
  Brain,
  Check,
  Clock,
  Coins,
  Database,
  Filter,
  Package,
  Thermometer,
  X,
} from "@wso2/oxygen-ui-icons-react";
import { scoreColor } from "@agent-management-platform/views";

interface BasicInfoSectionProps {
  span: Span;
  evaluatorScores?: EvaluatorScoreWithMonitor[];
}

function formatDuration(durationInNanos: number) {
  if (durationInNanos > 1000 * 1000 * 1000) {
    return `${(durationInNanos / (1000 * 1000 * 1000)).toFixed(2)}s`;
  }
  if (durationInNanos > 1000 * 1000) {
    return `${(durationInNanos / (1000 * 1000)).toFixed(2)}ms`;
  }
  return `${(durationInNanos / 1000).toFixed(2)}μs`;
}

export function BasicInfoSection({ span, evaluatorScores }: BasicInfoSectionProps) {
  // Extract fields from data based on kind
  const { kind, data } = span.ampAttributes || {};
  let model: string | undefined;
  let vendor: string | undefined;
  let tokenUsage:
    | { inputTokens: number; outputTokens: number; totalTokens: number }
    | undefined;
  let temperature: number | undefined;
  let framework: string | undefined;
  let vectorDB: string | undefined;
  let topK: number | undefined;

  if (kind === "llm" && data) {
    const llmData = data as LLMData;
    model = llmData.model;
    vendor = llmData.vendor;
    tokenUsage = llmData.tokenUsage;
    temperature = llmData.temperature;
  } else if (kind === "agent" && data) {
    const agentData = data as AgentData;
    model = agentData.model;
    framework = agentData.framework;
    tokenUsage = agentData.tokenUsage;
  } else if (kind === "embedding" && data) {
    const embeddingData = data as EmbeddingData;
    model = embeddingData.model;
    vendor = embeddingData.vendor;
    tokenUsage = embeddingData.tokenUsage;
  } else if (kind === "retriever" && data) {
    const retrieverData = data as RetrieverData;
    vectorDB = retrieverData.vectorDB;
    topK = retrieverData.topK;
  }

  // Format model display with vendor prefix if available
  const modelDisplay = vendor && model ? `${vendor}/${model}` : model;

  // Build evaluator label lookup (disambiguate duplicate evaluator names across monitors)
  const evalNameCounts = new Map<string, number>();
  if (evaluatorScores?.length) {
    for (const e of evaluatorScores) {
      evalNameCounts.set(e.evaluatorName, (evalNameCounts.get(e.evaluatorName) ?? 0) + 1);
    }
  }
  const getEvalLabel = (ev: EvaluatorScoreWithMonitor): string => {
    const hasDuplicate = (evalNameCounts.get(ev.evaluatorName) ?? 0) > 1;
    return hasDuplicate ? `${ev.monitorName}/${ev.evaluatorName}` : ev.evaluatorName;
  };

  return (
    <Stack spacing={1} direction="row" flexWrap="wrap" useFlexGap alignItems="center">
        {span.ampAttributes?.status?.error && (
          <Tooltip
            title={
              span.ampAttributes?.status?.errorType ||
              "Failed to execute the span"
            }
          >
            <Chip
              icon={<X size={16} />}
              size="small"
              variant="outlined"
              label={span.ampAttributes?.status?.errorType || "Failed"}
              color="error"
            />
          </Tooltip>
        )}
        {!span.ampAttributes?.status?.error && (
          <Chip
            icon={<Check size={16} />}
            size="small"
            variant="outlined"
            label={"Success"}
            color="success"
          />
        )}
        {span.durationInNanos != null && (
          <Tooltip title={"Execution duration"}>
            <Chip
              icon={<Clock size={16} />}
              size="small"
              variant="outlined"
              label={formatDuration(span.durationInNanos)}
            />
          </Tooltip>
        )}
        {framework && (
          <Tooltip title={"Framework"}>
            <Chip
              icon={<Package size={16} />}
              size="small"
              variant="outlined"
              label={framework}
            />
          </Tooltip>
        )}
        {modelDisplay && (
          <Tooltip title={"Model used"}>
            <Chip
              icon={<Brain size={16} />}
              size="small"
              variant="outlined"
              label={modelDisplay}
            />
          </Tooltip>
        )}
        {vectorDB && (
          <Tooltip title={"Vector database"}>
            <Chip
              icon={<Database size={16} />}
              size="small"
              variant="outlined"
              label={vectorDB}
            />
          </Tooltip>
        )}
        {topK !== undefined && (
          <Tooltip title={"Top K results"}>
            <Chip
              icon={<Filter size={16} />}
              size="small"
              variant="outlined"
              label={`Top ${topK}`}
            />
          </Tooltip>
        )}
        {tokenUsage && (
          <Tooltip
            title={`${tokenUsage.inputTokens} input tokens, ${tokenUsage.outputTokens} output tokens`}
          >
            <Chip
              icon={<Coins size={16} />}
              size="small"
              variant="outlined"
              label={tokenUsage.totalTokens}
            />
          </Tooltip>
        )}
        {temperature !== undefined && (
          <Tooltip title={"Temperature"}>
            <Chip
              icon={<Thermometer size={16} />}
              size="small"
              variant="outlined"
              label={temperature}
            />
          </Tooltip>
        )}
        {evaluatorScores && evaluatorScores.length > 0 &&
          evaluatorScores.map((ev, idx) => {
            const label = getEvalLabel(ev);

            if (ev.score != null) {
              const color = scoreColor(ev.score);
              return (
                <Tooltip key={idx} title={ev.explanation || "No explanation"}>
                  <Chip
                    size="small"
                    variant="outlined"
                    label={`${label}: ${(ev.score * 100).toFixed(1)}%`}
                    sx={{
                      color,
                      borderColor: color,
                    }}
                  />
                </Tooltip>
              );
            }
            return (
              <Tooltip key={idx} title={ev.skipReason || "Skipped"}>
                <Chip
                  size="small"
                  variant="outlined"
                  label={`${label}: Skipped`}
                  sx={{ opacity: 0.6, fontStyle: "italic" }}
                />
              </Tooltip>
            );
          })}
      </Stack>
  );
}

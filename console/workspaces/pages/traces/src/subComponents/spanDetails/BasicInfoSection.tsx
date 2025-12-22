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

import { Span } from "@agent-management-platform/types";
import { Chip, Stack, Tooltip } from "@wso2/oxygen-ui";
import {
  Brain,
  Check,
  Clock,
  Coins,
  Thermometer,
  X,
} from "@wso2/oxygen-ui-icons-react";

interface BasicInfoSectionProps {
  span: Span;
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

export function BasicInfoSection({ span }: BasicInfoSectionProps) {
  return (
    <Stack spacing={1} direction="row">
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
      {span.startTime && (
        <Tooltip title={"Execution duration"}>
          <Chip
            icon={<Clock size={16} />}
            size="small"
            variant="outlined"
            label={formatDuration(span.durationInNanos)}
          />
        </Tooltip>
      )}
      {span.ampAttributes?.model && (
        <Tooltip title={"Model used"}>
          <Chip
            icon={<Brain size={16} />}
            size="small"
            variant="outlined"
            label={span.ampAttributes?.model}
          />
        </Tooltip>
      )}
      {span.ampAttributes?.tokenUsage && (
        <Tooltip
          title={`Used ${span.ampAttributes?.tokenUsage.inputTokens} input tokens, ${span.ampAttributes?.tokenUsage.outputTokens} output tokens`}
        >
          <Chip
            icon={<Coins size={16} />}
            size="small"
            variant="outlined"
            label={span.ampAttributes?.tokenUsage.totalTokens}
          />
        </Tooltip>
      )}
      {span.ampAttributes?.temperature && (
        <Tooltip title={"Temperature"}>
          <Chip
            icon={<Thermometer size={16} />}
            size="small"
            variant="outlined"
            label={span.ampAttributes?.temperature}
          />
        </Tooltip>
      )}
    </Stack>
  );
}

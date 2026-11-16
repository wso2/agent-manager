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

import { EvaluatorScoreWithMonitor } from "@agent-management-platform/types";
import { Card, CardContent, Chip, Stack, Typography } from "@wso2/oxygen-ui";
import { scoreColor, MarkdownView } from "@agent-management-platform/views";

interface ScoresSectionProps {
  evaluatorScores: EvaluatorScoreWithMonitor[];
}

export function ScoresSection({ evaluatorScores }: ScoresSectionProps) {
  // Build evaluator label lookup (disambiguate duplicate evaluator names across monitors)
  const evalNameCounts = new Map<string, number>();
  for (const e of evaluatorScores) {
    evalNameCounts.set(e.evaluatorName, (evalNameCounts.get(e.evaluatorName) ?? 0) + 1);
  }
  const getEvalLabel = (ev: EvaluatorScoreWithMonitor): string => {
    const hasDuplicate = (evalNameCounts.get(ev.evaluatorName) ?? 0) > 1;
    return hasDuplicate ? `${ev.monitorName}/${ev.evaluatorName}` : ev.evaluatorName;
  };

  return (
    <Stack spacing={2} pt={1}>
      {evaluatorScores.map((ev, idx) => {
        const label = getEvalLabel(ev);
        const isSkipped = ev.score == null;
        const color = !isSkipped ? scoreColor(ev.score!) : undefined;

        return (
          <Card key={idx} variant="outlined">
            <CardContent>
              <Stack spacing={1.5}>
                <Stack direction="row" spacing={1} alignItems="center">
                  <Typography variant="subtitle2">{label}</Typography>
                  {!isSkipped ? (
                    <Chip
                      size="small"
                      variant="outlined"
                      label={`${(ev.score! * 100).toFixed(1)}%`}
                      sx={{ ml: "auto", color, borderColor: color }}
                    />
                  ) : (
                    <Chip
                      size="small"
                      variant="outlined"
                      label="Skipped"
                      sx={{ ml: "auto", opacity: 0.6, fontStyle: "italic" }}
                    />
                  )}
                </Stack>
                {ev.explanation && <MarkdownView content={ev.explanation} />}
                {isSkipped && ev.skipReason && (
                  <Typography variant="body2" color="text.secondary" sx={{ fontStyle: "italic" }}>
                    {ev.skipReason}
                  </Typography>
                )}
              </Stack>
            </CardContent>
          </Card>
        );
      })}
    </Stack>
  );
}

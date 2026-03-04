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

import React from "react";
import {
  Box,
  Card,
  CardContent,
  Chip,
  Divider,
  Stack,
  Typography,
  LinearProgress,
} from "@wso2/oxygen-ui";
import { Activity } from "@wso2/oxygen-ui-icons-react";
import { type EvaluationLevel } from "@agent-management-platform/types";
import { LEVEL_CONFIG, levelChipSx } from "./levelConfig";

export interface LevelSummary {
  level: EvaluationLevel;
  evaluatorCount: number;
  totalCount: number;
  skippedCount: number;
}

interface EvaluationSummaryCardProps {
  levels: LevelSummary[];
  averageScoreValue: string;
  averageScoreProgress: number;
}

const EvaluationSummaryCard: React.FC<EvaluationSummaryCardProps> = ({
  levels,
  averageScoreValue,
  averageScoreProgress,
}) => {
  const totalCount = levels.reduce((s, l) => s + l.totalCount, 0);

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
        >
          <Typography variant="subtitle1">Evaluation Summary</Typography>
        </Stack>
        {levels.length === 0 ? (
          <Box
            display="flex"
            flexDirection="column"
            alignItems="center"
            justifyContent="center"
            py={4}
            gap={1}
          >
            <Activity size={36} />
            <Typography variant="body2" fontWeight={500}>
              No evaluation data
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              textAlign="center"
            >
              Scores will appear here once evaluations complete.
            </Typography>
          </Box>
        ) : (
          <Stack direction="row" spacing={2}>
            <Stack spacing={1} width="40%">
              <Typography variant="caption" color="text.secondary">
                Average Score
              </Typography>
              <Stack spacing={2}>
                <Typography variant="h3">{averageScoreValue}</Typography>
                <LinearProgress
                  variant="determinate"
                  value={averageScoreProgress}
                />
              </Stack>
              <Typography variant="caption" color="text.secondary">
                {totalCount.toLocaleString()} total evaluations
              </Typography>
            </Stack>
            <Divider orientation="vertical" flexItem />
            <Stack spacing={1.5} flex={1}>
              {levels.map((lvl) => {
                const cfg = LEVEL_CONFIG[lvl.level];
                const skipPct = lvl.totalCount > 0
                  ? ((lvl.skippedCount / lvl.totalCount) * 100).toFixed(1)
                  : "0";
                return (
                  <Stack key={lvl.level} direction="row" alignItems="center" spacing={1.5}>
                    <Chip
                      label={cfg.label}
                      size="small"
                      sx={{ ...levelChipSx(cfg), fontSize: "0.75rem", height: 24, minWidth: 80 }}
                    />
                    <Stack spacing={0} flex={1}>
                      <Stack direction="row" alignItems="baseline" spacing={0.5}>
                        <Typography variant="body1" fontWeight={500} sx={{ fontSize: "1.1rem" }}>
                          {lvl.totalCount.toLocaleString()}
                        </Typography>
                        <Typography variant="caption" color="text.secondary">
                          {cfg.unit}
                        </Typography>
                      </Stack>
                      <Typography variant="caption" color="text.secondary">
                        {lvl.evaluatorCount} evaluator{lvl.evaluatorCount !== 1 ? "s" : ""}
                        {lvl.skippedCount > 0 ? ` \u00b7 ${lvl.skippedCount.toLocaleString()} skipped (${skipPct}%)` : ""}
                      </Typography>
                    </Stack>
                  </Stack>
                );
              })}
            </Stack>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
};

export default EvaluationSummaryCard;

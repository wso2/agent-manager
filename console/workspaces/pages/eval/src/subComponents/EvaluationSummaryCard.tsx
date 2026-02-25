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
  Divider,
  Stack,
  Typography,
  Tooltip,
  LinearProgress,
} from "@wso2/oxygen-ui";
import { Activity } from "@wso2/oxygen-ui-icons-react";

export interface EvaluationSummaryItem {
  label: string;
  value: string;
  helper: string;
  rate?: number;
}

interface EvaluationSummaryCardProps {
  items: EvaluationSummaryItem[];
  averageScoreValue: string;
  averageScoreProgress: number;
}

const EvaluationSummaryCard: React.FC<EvaluationSummaryCardProps> = ({
  items,
  averageScoreValue,
  averageScoreProgress,
}) => {

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
        {items.length === 0 ? (
          <Box
            display="flex"
            flexDirection="column"
            alignItems="center"
            justifyContent="center"
            py={4}
            gap={1}
          >
            <Activity size={48} />
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
            <Stack spacing={1} width="50%">
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
            </Stack>
            <Divider orientation="vertical" flexItem />
            <Stack spacing={2}>
              {items.map((item) => (
                <Stack key={item.label}>
                  <Stack spacing={0.5}>
                    <Typography variant="caption" color="text.secondary">
                      {item.label}
                    </Typography>
                    <Tooltip title={item.helper}>
                      <Typography variant="h5">{item.value}</Typography>
                    </Tooltip>
                  </Stack>
                </Stack>
              ))}
            </Stack>
          </Stack>
        )}
      </CardContent>
    </Card>
  );
};

export default EvaluationSummaryCard;

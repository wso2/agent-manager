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

import React, { useMemo } from "react";
import {
  Box,
  Card,
  CardContent,
  Skeleton,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@wso2/oxygen-ui";
import { Activity } from "@wso2/oxygen-ui-icons-react";
import { type GroupedScoresResponse } from "@agent-management-platform/types";
import ScoreChip from "./ScoreChip";

interface ScoreBreakdownCardProps {
  level: "agent" | "llm";
  data: GroupedScoresResponse | undefined;
  isLoading: boolean;
}

const CARD_TITLES: Record<"agent" | "llm", string> = {
  agent: "Score Breakdown by Agent",
  llm: "Score Breakdown by Model",
};

const ScoreBreakdownCard: React.FC<ScoreBreakdownCardProps> = ({
  level,
  data,
  isLoading,
}) => {
  const title = CARD_TITLES[level];

  // Collect unique evaluator names across all groups
  const evaluatorNames = useMemo(() => {
    if (!data?.groups) return [];
    const nameSet = new Set<string>();
    data.groups.forEach((g) =>
      g.evaluators.forEach((e) => nameSet.add(e.evaluatorName)),
    );
    return Array.from(nameSet).sort();
  }, [data]);

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" alignItems="center" spacing={1} mb={2}>
          <Typography variant="subtitle1">{title}</Typography>
        </Stack>

        {isLoading ? (
          <Skeleton variant="rounded" height={200} />
        ) : !data?.groups?.length ? (
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
              No {level} span data
            </Typography>
            <Typography
              variant="caption"
              color="text.secondary"
              textAlign="center"
            >
              {level === "agent"
                ? "Agent-level scores will appear once agent span evaluators run."
                : "LLM-level scores will appear once LLM span evaluators run."}
            </Typography>
          </Box>
        ) : (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell sx={{ fontWeight: 600 }}>
                    {level === "agent" ? "Agent" : "Model"}
                  </TableCell>
                  {evaluatorNames.map((name) => (
                    <TableCell
                      key={name}
                      align="center"
                      sx={{ fontWeight: 600 }}
                    >
                      {name}
                    </TableCell>
                  ))}
                  <TableCell align="right" sx={{ fontWeight: 600 }}>
                    Count
                  </TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {data.groups.map((group) => {
                  const evalMap = new Map(
                    group.evaluators.map((e) => [e.evaluatorName, e]),
                  );
                  const totalCount = group.evaluators.reduce(
                    (s, e) => s + e.count,
                    0,
                  );
                  return (
                    <TableRow key={group.label}>
                      <TableCell>
                        <Typography variant="body2" fontWeight={500}>
                          {group.label}
                        </Typography>
                      </TableCell>
                      {evaluatorNames.map((name) => {
                        const ev = evalMap.get(name);
                        const allSkipped = !ev || ev.count === ev.skippedCount;
                        if (allSkipped) {
                          return (
                            <TableCell key={name} align="center">
                              <Typography
                                variant="caption"
                                color="text.secondary"
                              >
                                –
                              </Typography>
                            </TableCell>
                          );
                        }
                        return (
                          <TableCell key={name} align="center">
                            <ScoreChip score={ev.mean} />
                          </TableCell>
                        );
                      })}
                      <TableCell align="right">
                        <Typography variant="body2">
                          {totalCount.toLocaleString()}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </CardContent>
    </Card>
  );
};

export default ScoreBreakdownCard;

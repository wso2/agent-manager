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

import { SpanStatus } from "@agent-management-platform/types";
import { Card, CardContent, Divider, Stack, Typography } from "@wso2/oxygen-ui";
import { getStatusMessageDetails } from "./statusMessage";

interface StatusSectionProps {
  status: SpanStatus;
}

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <Stack direction="row" spacing={1}>
      <Typography variant="body2" color="text.secondary" sx={{ minWidth: 140 }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ wordBreak: "break-word" }}>
        {value}
      </Typography>
    </Stack>
  );
}

export function StatusSection({ status }: StatusSectionProps) {
  const details = getStatusMessageDetails(status.message ?? "");
  const heading = details.type || status.errorType || "Status";
  const hasDetailRows = !!(details.direction || details.interveningGuardrail);

  return (
    <Stack spacing={2} pt={1}>
      <Card variant="outlined">
        <CardContent>
          <Stack spacing={1.5}>
            <Stack direction="row" spacing={1} alignItems="center">
              <Typography variant="subtitle2">{heading}</Typography>
            </Stack>
            {details.actionReason && (
              <Typography variant="body2">{details.actionReason}</Typography>
            )}

            {hasDetailRows && (
              <Stack spacing={0.5}>
                {details.interveningGuardrail && (
                  <DetailRow label="Guardrail" value={details.interveningGuardrail} />
                )}
                {details.direction && (
                  <DetailRow label="Direction" value={details.direction} />
                )}
              </Stack>
            )}

            {details.raw && (
              <>
                <Divider />
                <Typography variant="caption" color="text.secondary">
                  Message
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    whiteSpace: "pre-wrap",
                    wordBreak: "break-word",
                    fontFamily: "monospace",
                  }}
                >
                  {details.raw}
                </Typography>
              </>
            )}
          </Stack>
        </CardContent>
      </Card>
    </Stack>
  );
}

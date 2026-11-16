/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Card, CardContent, Stack, Typography } from "@wso2/oxygen-ui";

export interface InfoCardProps {
  label: string;
  value: string;
  /** Renders the value in a monospace font. Defaults to true. */
  monospace?: boolean;
}

/**
 * A small read-only label/value tile used on overview pages (e.g. DNS
 * prefix, namespace, environment name).
 */
export function InfoCard({ label, value, monospace = true }: InfoCardProps) {
  return (
    <Card variant="outlined" sx={{ height: "100%" }}>
      <CardContent sx={{ p: 2, "&:last-child": { pb: 2 } }}>
        <Stack spacing={0.5}>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
            {label}
          </Typography>
          <Typography
            variant="body2"
            sx={{ fontFamily: monospace ? "monospace" : undefined, wordBreak: "break-all" }}
          >
            {value}
          </Typography>
        </Stack>
      </CardContent>
    </Card>
  );
}

export default InfoCard;

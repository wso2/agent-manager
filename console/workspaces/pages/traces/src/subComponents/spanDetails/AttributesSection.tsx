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

import { NoDataFound } from "@agent-management-platform/views";
import {
  Box,
  Card,
  CardContent,
  Stack,
  Typography,
  alpha,
  useTheme,
} from "@wso2/oxygen-ui";
import { ChartArea } from "@wso2/oxygen-ui-icons-react";
import { useCallback } from "react";

interface AttributesSectionProps {
  attributes?: Record<string, unknown>;
}

export function AttributesSection({ attributes }: AttributesSectionProps) {
  const theme = useTheme();

  const isJsonObject = useCallback((value: unknown): boolean => {
    if (value === null || value === undefined) return false;

    if (typeof value === "object" || Array.isArray(value)) return true;

    if (typeof value === "string") {
      try {
        const parsed = JSON.parse(value);
        return typeof parsed === "object" && parsed !== null;
      } catch {
        return false;
      }
    }

    return false;
  }, []);

  const renderAttributeValue = useCallback((value: unknown): string => {
    if (value === null) return "null";
    if (value === undefined) return "undefined";

    if (typeof value === "object" || Array.isArray(value)) {
      try {
        return JSON.stringify(value, null, 2);
      } catch {
        return String(value);
      }
    }

    if (typeof value === "string") {
      try {
        const parsed = JSON.parse(value);
        if (typeof parsed === "object" && parsed !== null) {
          return JSON.stringify(parsed, null, 2);
        }
      } catch {
        // Not valid JSON, return as-is
      }
    }

    return String(value);
  }, []);

  if (!attributes || Object.keys(attributes).length === 0) {
    return <NoDataFound
      message="No attributes found"
      iconElement={ChartArea}
      subtitle="No attributes found"
      disableBackground
    />
  }

  return (
    <Stack direction="column" spacing={2}>
      {Object.entries(attributes).map(([key, value]) => (
        <Box key={key}>
          <Typography variant="h6">{key}</Typography>
          <Card
            variant="outlined"
            sx={{
              maxHeight: isJsonObject(value) ? 375 : "auto",
              overflow: "auto",
              bgcolor:
                theme.palette.mode === "dark"
                  ? alpha(theme.palette.common.black, 0.2)
                  : alpha(theme.palette.common.black, 0.03),
            }}
          >
            <CardContent sx={{ "&:last-child": { pb: 1.5 } }}>
              <Typography
                variant="caption"
                sx={{
                  fontFamily: "monospace",
                  whiteSpace: "pre-wrap",
                  wordBreak: "break-word",
                }}
              >
                {renderAttributeValue(value)}
              </Typography>
            </CardContent>
          </Card>
        </Box>
      ))}
    </Stack>
  );
}

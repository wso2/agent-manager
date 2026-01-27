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

import { useState, useEffect } from "react";
import { Box, Typography, Button, Select, MenuItem, CircularProgress } from "@wso2/oxygen-ui";
import { CodeBlock } from "@agent-management-platform/shared-component";
import { useGenerateAgentToken } from "@agent-management-platform/api-client";

interface TokenGenerationStepProps {
  stepNumber: number;
  orgName: string;
  projName: string;
  agentName: string;
  environment?: string;
  onTokenGenerated: (token: string) => void;
}

const DURATION_OPTIONS = [
  { label: "30 days", value: "720h" },
  { label: "90 days", value: "2160h" },
  { label: "6 months", value: "4320h" },
  { label: "1 year", value: "8760h" },
  { label: "2 years", value: "17520h" },
];

export const TokenGenerationStep = ({
  stepNumber,
  orgName,
  projName,
  agentName,
  environment,
  onTokenGenerated,
}: TokenGenerationStepProps) => {
  const [duration, setDuration] = useState<string>("8760h"); // Default to 1 year

  const { data, isFetching, error, refetch } = useGenerateAgentToken(
    {
      agentName,
      projName,
      orgName,
    },
    {
      expires_in: duration,
    },
    environment ? { environment } : undefined
  );

  // Call the callback when token is generated
  useEffect(() => {
    if (data?.token) {
      onTokenGenerated(data.token);
    }
  }, [data?.token, onTokenGenerated]);

  const displayToken = data?.token || "ey***";
  const codeSnippet = `API Key="${displayToken}"`;

  return (
    <Box display="flex" gap={1} flexDirection="column">
      <Box display="flex" alignItems="center" gap={1} justifyContent="space-between">
        <Box display="flex" alignItems="center" gap={1}>
          <Box
            sx={{
              gap: 2,
              width: 20,
              height: 20,
              borderRadius: "50%",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              bgcolor: (theme) => theme.palette.primary.main,
              color: "primary.contrastText",
              fontWeight: 600,
            }}
          >
            <Typography variant="body2" fontWeight={600}>
              {stepNumber}
            </Typography>
          </Box>
          <Typography variant="body1">Generate API Key</Typography>
        </Box>

        <Box display="flex" gap={1} alignItems="center">
          <Typography variant="body2" color="textSecondary">
            Token Duration
          </Typography>
          <Select
            value={duration}
            onChange={(e) => setDuration(e.target.value as string)}
            size="small"
            disabled={ isFetching}
            sx={{ minWidth: 100 }}
          >
            {DURATION_OPTIONS.map((option) => (
              <MenuItem key={option.value} value={option.value}>
                {option.label}
              </MenuItem>
            ))}
          </Select>

          <Button
            variant="contained"
            onClick={()=>refetch()}
            disabled={ isFetching}
            startIcon={isFetching ? <CircularProgress size={16} /> : undefined}
            size="small"
          >
            {isFetching ? "Generating..." : data?.token ? "Generated" : "Generate"}
          </Button>
        </Box>
      </Box>

      <Box display="flex" flexDirection="column" gap={1}>
        {error && (
          <Typography variant="body2" color="error">
            Failed to generate token. Please try again.
          </Typography>
        )}

        <CodeBlock code={codeSnippet} language="bash" fieldId="api-key" />

        <Typography variant="body2" color="textSecondary">
          {data?.token
            ? "Token generated successfully. Copy it now as you won't be able to see it again."
            : "Generate a token to authenticate your traces."}
        </Typography>
      </Box>
    </Box>
  );
};

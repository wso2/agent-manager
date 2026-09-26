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

import { useState, useEffect, useRef } from "react";
import { Box, Typography, Button, CircularProgress } from "@wso2/oxygen-ui";
import { KeyRound } from "@wso2/oxygen-ui-icons-react";
import { CodeBlock, useConfirmationDialog } from "@agent-management-platform/shared-component";
import { TokenExpirySelector, DEFAULT_TOKEN_EXPIRY } from "@agent-management-platform/views";
import { useGenerateAgentToken } from "@agent-management-platform/api-client";
import { StepNumberBadge } from "./StepNumberBadge";

interface TokenGenerationStepProps {
  stepNumber: number;
  orgName: string;
  projName: string;
  agentName: string;
  environment?: string;
  onTokenGenerated: (token: string) => void;
  // When true, mint a token once automatically on first open (right after agent creation).
  // Subsequent opens require an explicit Generate click.
  autoGenerate?: boolean;
}

export const TokenGenerationStep = ({
  stepNumber,
  orgName,
  projName,
  agentName,
  environment,
  onTokenGenerated,
  autoGenerate = false,
}: TokenGenerationStepProps) => {
  const [duration, setDuration] = useState<string>(DEFAULT_TOKEN_EXPIRY);
  const [enabled, setEnabled] = useState(false);
  const autoGenFired = useRef(false);
  const { addConfirmation } = useConfirmationDialog();

  // Session-scoped guard so a reopen never silently re-mints (#1140). Scoped per
  // (org, project, agent, environment).
  const sessionKey = `amp-token-generated:${orgName}:${projName}:${agentName}:${environment ?? ""}`;

  const { data, isFetching, error, refetch } = useGenerateAgentToken(
    { agentName, projName, orgName },
    { expires_in: duration },
    environment ? { environment } : undefined,
    enabled,
  );
  const token = data?.token ?? null;

  useEffect(() => {
    if (data?.token) onTokenGenerated(data.token);
  }, [data?.token, onTokenGenerated]);

  // Mints once: setEnabled(true) triggers the first fetch; a later regenerate uses refetch().
  const generate = () => {
    try {
      sessionStorage.setItem(sessionKey, "1");
    } catch {
      // sessionStorage may be unavailable (private mode); the query still works.
    }
    if (enabled) {
      refetch();
    } else {
      setEnabled(true);
    }
  };

  // Auto-generate exactly once on first open after creation.
  useEffect(() => {
    if (!autoGenerate || autoGenFired.current) return;
    let alreadyGenerated = false;
    try {
      alreadyGenerated = sessionStorage.getItem(sessionKey) === "1";
    } catch {
      alreadyGenerated = false;
    }
    if (!alreadyGenerated) {
      autoGenFired.current = true;
      generate();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoGenerate, sessionKey]);

  const handleGenerateClick = () => {
    let alreadyGenerated = false;
    try {
      alreadyGenerated = sessionStorage.getItem(sessionKey) === "1";
    } catch {
      alreadyGenerated = false;
    }
    // Confirm whenever a token was already minted this session (in-state token or session flag).
    if (token || alreadyGenerated) {
      // Regenerating: confirm first, since a new token must be reconfigured wherever it is used.
      addConfirmation({
        analytics: { entity: "agent-api-key", action: "regenerate" },
        title: "Regenerate API key?",
        description:
          "A new API key will be generated. Previously configured keys remain valid until they expire.",
        confirmButtonText: "Regenerate",
        onConfirm: generate,
      });
      return;
    }
    generate();
  };

  const displayToken = token || "ey***";

  return (
    <Box display="flex" gap={1} flexDirection="column">
      <Box display="flex" alignItems="center" gap={1} justifyContent="space-between">
        <Box display="flex" alignItems="center" gap={1}>
          <StepNumberBadge stepNumber={stepNumber} />
          <Typography variant="body1">Generate API Key</Typography>
        </Box>

        <Box display="flex" gap={1} alignItems="center">
          <Typography variant="body2" color="textSecondary">
            Token Duration
          </Typography>
          <TokenExpirySelector value={duration} onChange={setDuration} disabled={isFetching} />

          <Button
            variant="text"
            onClick={handleGenerateClick}
            disabled={isFetching}
            startIcon={isFetching ? <CircularProgress size={16} /> : <KeyRound size={16} />}
            size="small"
          >
            {isFetching ? "Generating..." : token ? "Regenerate" : "Generate"}
          </Button>
        </Box>
      </Box>

      <Box display="flex" flexDirection="column" gap={1}>
        {error ? (
          <Typography variant="body2" color="error">
            Failed to generate token. Please try again.
          </Typography>
        ) : null}

        <CodeBlock code={displayToken} language="bash" fieldId="api-key" />

        <Typography variant="body2" color="textSecondary">
          {token
            ? "Token generated successfully. Copy it now as you won't be able to see it again."
            : "Generate a token to authenticate your traces."}
        </Typography>
      </Box>
    </Box>
  );
};

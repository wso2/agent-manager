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

import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Grid,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";
import { INPUT_LIMITS } from "@agent-management-platform/types";
import { TextInput } from "@agent-management-platform/views";

// Gateway defaults applied when a value is omitted (RouteTimeoutMs / RouteIdleTimeoutMs).
export const DEFAULT_REQUEST_TIMEOUT = "60s";
export const DEFAULT_IDLE_TIMEOUT = "300s";

export interface ResilienceTimeoutFieldsProps {
  requestTimeout: string;
  onRequestTimeoutChange: (value: string) => void;
  onRequestTimeoutBlur?: () => void;
  requestTimeoutError?: string | null;
  idleTimeout: string;
  onIdleTimeoutChange: (value: string) => void;
  onIdleTimeoutBlur?: () => void;
  idleTimeoutError?: string | null;
  defaultExpanded?: boolean;
}

/** Request/idle timeout fields shared by the LLM provider and MCP proxy connection forms. */
export function ResilienceTimeoutFields({
  requestTimeout,
  onRequestTimeoutChange,
  onRequestTimeoutBlur,
  requestTimeoutError,
  idleTimeout,
  onIdleTimeoutChange,
  onIdleTimeoutBlur,
  idleTimeoutError,
  defaultExpanded = false,
}: ResilienceTimeoutFieldsProps) {
  return (
    <Accordion defaultExpanded={defaultExpanded} disableGutters variant="outlined">
      <AccordionSummary expandIcon={<ChevronDown size={18} />}>
        <Typography variant="subtitle2" fontWeight={600}>
          Resilience
        </Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, md: 6 }}>
            <TextInput
              label="Request Timeout"
              maxLength={INPUT_LIMITS.KEY}
              hideCharacterCount
              value={requestTimeout}
              onChange={(e) => onRequestTimeoutChange(e.target.value)}
              onBlur={onRequestTimeoutBlur}
              placeholder={DEFAULT_REQUEST_TIMEOUT}
              error={Boolean(requestTimeoutError)}
              helperText={requestTimeoutError}
            />
          </Grid>
          <Grid size={{ xs: 12, md: 6 }}>
            <TextInput
              label="Idle Timeout"
              maxLength={INPUT_LIMITS.KEY}
              hideCharacterCount
              value={idleTimeout}
              onChange={(e) => onIdleTimeoutChange(e.target.value)}
              onBlur={onIdleTimeoutBlur}
              placeholder={DEFAULT_IDLE_TIMEOUT}
              error={Boolean(idleTimeoutError)}
              helperText={idleTimeoutError}
            />
          </Grid>
        </Grid>
      </AccordionDetails>
    </Accordion>
  );
}

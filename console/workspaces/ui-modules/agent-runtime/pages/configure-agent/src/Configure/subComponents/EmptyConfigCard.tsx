/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import type { ReactNode } from "react";
import { Button, Card, CardContent, Stack, Typography } from "@wso2/oxygen-ui";

interface EmptyConfigCardProps {
  /** Explains what is missing for the selected environment. */
  message: string;
  actionLabel: string;
  actionIcon?: ReactNode;
  onAction: () => void;
  disabled?: boolean;
}

/**
 * A compact "nothing configured for this environment yet" card with a single
 * call-to-action button. Shared by the LLM and MCP agent-config view pages so
 * an environment with no provider/server still offers a way to add one.
 */
export function EmptyConfigCard({
  message,
  actionLabel,
  actionIcon,
  onAction,
  disabled,
}: EmptyConfigCardProps) {
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack spacing={1.5} alignItems="flex-start">
          <Typography variant="body2" color="text.secondary">
            {message}
          </Typography>
          <Button
            variant="outlined"
            size="small"
            startIcon={actionIcon}
            onClick={onAction}
            disabled={disabled}
          >
            {actionLabel}
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}

export default EmptyConfigCard;

/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useEffect, useState } from 'react';
import { Alert, Button, CircularProgress, Stack, Typography } from '@wso2/oxygen-ui';
import { useIssueTestAgentAPIKey } from '@agent-management-platform/api-client';
import { formatRelativeTime, monospaceInputSx } from '@agent-management-platform/shared-component';
import type {
  IssueTestAgentAPIKeyPathParams,
  IssueTestAgentAPIKeyResponse,
} from '@agent-management-platform/types';
import { TextInput } from '@agent-management-platform/views';

const ISSUE_FAILED = "Couldn't issue a test key. Try again.";
const GATEWAY_DISCONNECTED =
  'The gateway is not connected to the control plane right now. The API key has been stored but will only work once the gateway reconnects.';

export interface A2ATestKeyIssuerProps {
  params: IssueTestAgentAPIKeyPathParams;
  onKeyChange: (key: string | null) => void;
}

function IssueFailed() {
  return (
    <Typography variant="body2" sx={{ color: 'error.main' }}>
      {ISSUE_FAILED}
    </Typography>
  );
}

function IssuedKey({ issued }: { issued: IssueTestAgentAPIKeyResponse }) {
  return (
    <>
      <TextInput
        label="Test API key"
        value={issued.apiKey ?? ''}
        copyable
        copyTooltipText="Copy API key"
        size="small"
        fullWidth
        slotProps={{ input: { readOnly: true } }}
        sx={monospaceInputSx}
      />
      <Typography variant="caption" color="text.secondary">
        Your personal test key for this environment. It expires{' '}
        {formatRelativeTime(issued.expiresAt)}.
      </Typography>
      {issued.gatewayConnected === false && <Alert severity="warning">{GATEWAY_DISCONNECTED}</Alert>}
    </>
  );
}

/**
 * Issues the caller's personal test key on an explicit click only, because issuing rotates the
 * previous one on the backend. The last successful response outlives a failed or pending rotate.
 */
export function A2ATestKeyIssuer({ params, onKeyChange }: A2ATestKeyIssuerProps) {
  const { mutate, isPending, isError } = useIssueTestAgentAPIKey();
  const [issued, setIssued] = useState<IssueTestAgentAPIKeyResponse | null>(null);

  useEffect(() => {
    onKeyChange(issued?.apiKey ?? null);
  }, [issued, onKeyChange]);

  const issue = () => mutate(params, { onSuccess: setIssued });
  const spinner = isPending ? <CircularProgress size={16} /> : null;

  if (issued === null) {
    return (
      <Stack spacing={1} alignItems="flex-start">
        {isError && !isPending && <IssueFailed />}
        <Stack direction="row" spacing={1} alignItems="center">
          <Button size="small" variant="outlined" onClick={issue} disabled={isPending}>
            {isPending ? 'Issuing…' : isError ? 'Retry' : 'Issue test key'}
          </Button>
          {spinner}
        </Stack>
      </Stack>
    );
  }

  return (
    <Stack spacing={1}>
      <IssuedKey issued={issued} />
      {isError && !isPending && <IssueFailed />}
      <Stack direction="row" spacing={1} alignItems="center">
        <Button size="small" variant="outlined" onClick={issue} disabled={isPending}>
          Rotate
        </Button>
        {spinner}
      </Stack>
    </Stack>
  );
}

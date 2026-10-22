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

import { Stack, Typography } from '@wso2/oxygen-ui';
import { monospaceInputSx } from '@agent-management-platform/shared-component';
import { TextInput } from '@agent-management-platform/views';
import type { ConnectEndpoints } from './connectEndpoints';

export interface A2AConnectEndpointsProps {
  resolved: ConnectEndpoints;
}

function UrlRow({ label, url }: { label: string; url: string }) {
  return (
    <TextInput
      label={label}
      value={url}
      copyable
      size="small"
      fullWidth
      slotProps={{ input: { readOnly: true } }}
      sx={monospaceInputSx}
    />
  );
}

export function A2AConnectEndpoints({ resolved }: A2AConnectEndpointsProps) {
  const { endpoints, declared, cardUrl } = resolved;
  return (
    <Stack spacing={1}>
      {endpoints.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          The card declares no endpoints yet.
        </Typography>
      )}
      {endpoints.map(({ label, url }, index) => (
        <UrlRow key={`${index}-${url}`} label={label} url={url} />
      ))}
      {cardUrl !== '' && <UrlRow label="Agent card" url={cardUrl} />}
      {declared.length > 0 && (
        <Stack spacing={0.5}>
          <Typography variant="caption" color="text.secondary">
            Declared by the card
          </Typography>
          {declared.map(({ url, protocolBinding }, index) => (
            <Typography
              key={`${index}-${url}`}
              variant="caption"
              sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}
            >
              {protocolBinding ? `${url} · ${protocolBinding}` : url}
            </Typography>
          ))}
        </Stack>
      )}
    </Stack>
  );
}

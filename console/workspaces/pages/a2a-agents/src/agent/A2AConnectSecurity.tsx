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

import type { ReactNode } from 'react';
import { Box, Stack, Typography } from '@wso2/oxygen-ui';
import type {
  A2AConnectOAuth,
  A2AConnectSecurity as A2AConnectSecurityModel,
  IssueTestAgentAPIKeyPathParams,
} from '@agent-management-platform/types';
import { A2ATestKeyIssuer } from './A2ATestKeyIssuer';

export const DEFAULT_OAUTH_HEADER = 'Authorization';

/** Read in place of a missing `oauth` object so an older or malformed response still renders. */
export const EMPTY_OAUTH: A2AConnectOAuth = {
  issuers: [],
  audiences: [],
  headerName: '',
  authHeaderPrefix: '',
};

export interface A2AConnectSecurityProps {
  security: A2AConnectSecurityModel;
  isProduction: boolean;
  securitySchemes: string[];
  keyParams: IssueTestAgentAPIKeyPathParams;
  onKeyChange: (key: string | null) => void;
}

function Code({ children }: { children: ReactNode }) {
  return (
    <Box component="code" sx={{ fontFamily: 'monospace' }}>
      {children}
    </Box>
  );
}

function ApiKeySecurity({
  apiKeyHeader,
  isProduction,
  keyParams,
  onKeyChange,
}: Pick<A2AConnectSecurityProps, 'isProduction' | 'keyParams' | 'onKeyChange'> & {
  apiKeyHeader: string;
}) {
  return (
    <Stack spacing={1}>
      <Typography variant="body2">
        API key required in header <Code>{apiKeyHeader}</Code>
      </Typography>
      {isProduction ? (
        <Typography variant="body2" color="text.secondary">
          Keys for production are issued by the agent&apos;s owners.
        </Typography>
      ) : (
        <A2ATestKeyIssuer params={keyParams} onKeyChange={onKeyChange} />
      )}
    </Stack>
  );
}

function OAuthSecurity({ oauth }: { oauth: A2AConnectOAuth }) {
  const { issuers, audiences, headerName, authHeaderPrefix } = oauth;
  return (
    <Stack spacing={0.5}>
      <Typography variant="body2">
        OAuth access token required in header <Code>{headerName || DEFAULT_OAUTH_HEADER}</Code>
        {authHeaderPrefix && (
          <>
            {' with prefix '}
            <Code>{authHeaderPrefix}</Code>
          </>
        )}
      </Typography>
      <Typography variant="body2" color="text.secondary">
        Issuers:
      </Typography>
      {issuers.map(({ name, issuer }, index) => (
        <Typography key={`${index}-${name}`} variant="body2" sx={{ pl: 1 }}>
          {`${name} — ${issuer || 'not found'}`}
        </Typography>
      ))}
      <Typography variant="body2" color="text.secondary">
        {`Audiences: ${audiences.length > 0 ? audiences.join(', ') : 'any'}`}
      </Typography>
    </Stack>
  );
}

export function A2AConnectSecurity({
  security,
  isProduction,
  securitySchemes,
  keyParams,
  onKeyChange,
}: A2AConnectSecurityProps) {
  switch (security.mode) {
    case 'apiKey':
      return (
        <ApiKeySecurity
          apiKeyHeader={security.apiKeyHeader ?? ''}
          isProduction={isProduction}
          keyParams={keyParams}
          onKeyChange={onKeyChange}
        />
      );
    case 'oauth':
      return <OAuthSecurity oauth={security.oauth ?? EMPTY_OAUTH} />;
    case 'none':
      return <Typography variant="body2">No authentication required.</Typography>;
    case 'unmanaged': {
      const declared = securitySchemes.length > 0 ? securitySchemes.join(', ') : 'nothing';
      return (
        <Typography variant="body2">
          {`Security isn't managed by the platform. The card declares: ${declared}.`}
        </Typography>
      );
    }
    default:
      return (
        <Typography variant="body2">
          Security settings couldn&apos;t be determined for this environment.
        </Typography>
      );
  }
}

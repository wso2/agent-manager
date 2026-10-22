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

import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { Alert, Box, Button, CircularProgress, Stack, Typography } from '@wso2/oxygen-ui';
import { useGetA2AConnect } from '@agent-management-platform/api-client';
import type { ParsedA2ACard } from '@agent-management-platform/shared-component';
import type {
  A2ACatalogAgent,
  A2AConnectResponse,
  A2AConnectSecurity as A2AConnectSecurityModel,
  IssueTestAgentAPIKeyPathParams,
} from '@agent-management-platform/types';
import { A2AConnectEndpoints } from './A2AConnectEndpoints';
import { A2AConnectSecurity, DEFAULT_OAUTH_HEADER, EMPTY_OAUTH } from './A2AConnectSecurity';
import { A2AQuickStart } from './A2AQuickStart';
import { resolveConnectEndpoints } from './connectEndpoints';
import { manageSecurityHref } from './manageSecurityHref';
import {
  ACCESS_TOKEN_PLACEHOLDER,
  API_KEY_PLACEHOLDER,
  type SnippetAuth,
  type SnippetInputs,
} from './quickStartSnippets';

export interface A2AConnectSectionProps {
  orgId: string;
  entry: A2ACatalogAgent;
  environment: string;
  card: ParsedA2ACard;
}

function snippetAuth(
  security: A2AConnectSecurityModel,
  issuedKey: string | null,
): SnippetAuth | null {
  switch (security.mode) {
    case 'apiKey':
      return { header: security.apiKeyHeader ?? '', value: issuedKey ?? API_KEY_PLACEHOLDER };
    case 'oauth': {
      const oauth = security.oauth ?? EMPTY_OAUTH;
      const prefix = oauth.authHeaderPrefix ?? '';
      return {
        header: oauth.headerName || DEFAULT_OAUTH_HEADER,
        value: prefix ? `${prefix} ${ACCESS_TOKEN_PLACEHOLDER}` : ACCESS_TOKEN_PLACEHOLDER,
      };
    }
    default:
      return null;
  }
}

function PublicationWarning({ publication }: Pick<A2AConnectResponse, 'publication'>) {
  if (publication.status !== 'pending' && publication.status !== 'failed') return null;
  const detail = publication.lastError ? ` ${publication.lastError}` : '';
  return (
    <Alert severity="warning">{`The gateway may not yet enforce these settings.${detail}`}</Alert>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Stack spacing={1}>
      <Typography variant="subtitle2">{title}</Typography>
      {children}
    </Stack>
  );
}

interface ConnectDetailsProps {
  data: A2AConnectResponse;
  refreshFailed: boolean;
  card: ParsedA2ACard;
  keyParams: IssueTestAgentAPIKeyPathParams;
  issuedKey: string | null;
  onKeyChange: (key: string | null) => void;
}

function ConnectDetails({
  data,
  refreshFailed,
  card,
  keyParams,
  issuedKey,
  onKeyChange,
}: ConnectDetailsProps) {
  const resolved = resolveConnectEndpoints(data, card.interfaces);
  const inputs: SnippetInputs = {
    cardUrl: resolved.cardUrl,
    endpointUrl: resolved.snippetEndpoint,
    auth: snippetAuth(data.security, issuedKey),
    authUnmanaged: data.security.mode === 'unmanaged',
  };
  return (
    <Stack spacing={2}>
      {refreshFailed ? (
        <Alert severity="warning">Couldn&apos;t refresh; showing the last loaded details.</Alert>
      ) : null}
      <PublicationWarning publication={data.publication} />
      <Section title="Endpoint">
        <A2AConnectEndpoints resolved={resolved} />
      </Section>
      <Section title="Security">
        <A2AConnectSecurity
          security={data.security}
          isProduction={data.isProduction}
          securitySchemes={card.securitySchemes}
          keyParams={keyParams}
          onKeyChange={onKeyChange}
        />
      </Section>
      <Section title="Quick start">
        <A2AQuickStart inputs={inputs} />
      </Section>
    </Stack>
  );
}

/** The Connect drawer body for one environment; it owns the connect query and the issued key. */
export function A2AConnectSection({ orgId, entry, environment, card }: A2AConnectSectionProps) {
  const envParams = {
    orgName: orgId,
    projName: entry.projectName,
    agentName: entry.agentName,
    envId: environment,
  };
  const { data, isError } = useGetA2AConnect(envParams);
  const [issuedKey, setIssuedKey] = useState<string | null>(null);
  const securityHref = manageSecurityHref(
    entry.kind,
    orgId,
    entry.projectName,
    entry.agentName,
    environment,
  );

  let body: ReactNode;
  if (data) {
    body = (
      <ConnectDetails
        data={data}
        refreshFailed={isError}
        card={card}
        keyParams={envParams}
        issuedKey={issuedKey}
        onKeyChange={setIssuedKey}
      />
    );
  } else if (isError) {
    body = (
      <Typography variant="body2" sx={{ color: 'error.main' }}>
        Couldn&apos;t load connection details for this environment.
      </Typography>
    );
  } else {
    body = <CircularProgress size={16} />;
  }

  return (
    <Stack spacing={2}>
      <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
        <Button size="small" variant="text" component={Link} to={securityHref}>
          Manage security
        </Button>
      </Box>
      {body}
    </Stack>
  );
}

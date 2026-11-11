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

import { useMemo, type ReactNode } from "react";
import { Alert, Box, Card, Form, Link, Typography } from "@wso2/oxygen-ui";
import { Plug } from "@wso2/oxygen-ui-icons-react";
import { Link as RouterLink } from "react-router-dom";
import { useListEnvironmentIdentityProviders } from "@agent-management-platform/api-client";
import { CodeBlock, getAgentSecurityPath } from "@agent-management-platform/shared-component";
import { DrawerWrapper, DrawerHeader, DrawerContent, TextInput } from "@agent-management-platform/views";

/** Builds a multi-line `curl ... \` command from its individual flag/arg parts. */
function buildCurl(parts: string[]): string {
  return parts.join(" \\\n  ");
}

/** client_credentials token request for one identity provider. */
function buildTokenCurl(tokenEndpoint?: string): string {
  const curl = buildCurl([
    `curl -X POST '${tokenEndpoint ?? "<token-endpoint>"}'`,
    "-H 'Content-Type: application/x-www-form-urlencoded'",
    "-d 'grant_type=client_credentials'",
    "-d 'client_id=<client-id>'",
    "-d 'client_secret=<client-secret>'",
  ]);
  return tokenEndpoint
    ? curl
    : `# Replace <token-endpoint> with your identity provider's token endpoint\n${curl}`;
}

export type AuthMode = "none" | "apikey" | "oauth";

interface ConsumerConfigDrawerProps {
  open: boolean;
  onClose: () => void;
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
  invokeUrl?: string;
  authMode: AuthMode;
  /** Human-readable auth summary shown next to Invoke URL, e.g. "OAuth2 (Bearer)". */
  authLabel: string;
  /** Literal header a caller must send, e.g. "x-api-key: <your-api-key>". */
  authHeaderExample: string;
  /** Identity provider names configured on the environment's OAuth security. */
  oauthIssuers: string[];
}

/**
 * Read-only "how do I call this agent" reference for API consumers: the invoke
 * URL, the auth header they must send, and — for OAuth — the identity
 * provider endpoints (issuer/JWKS) their token must be validated against.
 * Identity providers are only fetched when there's something to resolve
 * (OAuth mode with at least one configured issuer), the drawer is open.
 */
export function ConsumerConfigDrawer({
  open,
  onClose,
  orgId,
  projectId,
  agentId,
  envId,
  invokeUrl,
  authMode,
  authLabel,
  authHeaderExample,
  oauthIssuers,
}: ConsumerConfigDrawerProps) {
  const needsIdentityProviders = open && authMode === "oauth" && oauthIssuers.length > 0;

  const securityPath = useMemo(
    () => getAgentSecurityPath(orgId, projectId, agentId, envId),
    [orgId, projectId, agentId, envId],
  );

  const { data: idpResp, isLoading: isLoadingProviders } = useListEnvironmentIdentityProviders({
    orgName: needsIdentityProviders ? orgId : "",
    environmentId: needsIdentityProviders ? envId : undefined,
  });

  // `found: false` means the name in oauthConfig.issuers no longer matches any
  // identity provider on this environment (e.g. it was deleted) — surfaced
  // distinctly below rather than silently rendered as a still-loading "—".
  const issuerDetails = useMemo(() => {
    const byName = new Map((idpResp?.list ?? []).map((p) => [p.name, p]));
    return oauthIssuers.map((name) => {
      const provider = byName.get(name);
      return { name, issuer: provider?.issuer, jwksUri: provider?.jwksUri, found: !!provider };
    });
  }, [idpResp, oauthIssuers]);

  const invokeCurl = useMemo(() => {
    const parts = [
      `curl -X POST '${invokeUrl ?? "<invoke-url>"}'`,
      "-H 'Content-Type: application/json'",
    ];
    if (authMode !== "none") {
      parts.push(`-H '${authHeaderExample.replace("<token>", "<access-token>")}'`);
    }
    parts.push("-d '{}'");
    return buildCurl(parts);
  }, [invokeUrl, authMode, authHeaderExample]);

  const callAgentCopy: ReactNode = authMode === "oauth"
    ? "Call the agent with the access token obtained above."
    : authMode === "apikey"
      ? (
        <>
          Call the agent. Generate an API key on the{" "}
          <Link component={RouterLink} to={securityPath}>Credentials page</Link>.
        </>
      )
      : "Call the agent.";

  return (
    <DrawerWrapper open={open} onClose={onClose} maxWidth={600}>
      <DrawerHeader icon={<Plug size={24} />} title="Consumer Configuration" onClose={onClose} />
      <DrawerContent>
        <Form.Section>
          <Form.Subheader>Connection Details</Form.Subheader>
          <Form.Stack spacing={2}>
            <TextInput
              label="Invoke URL"
              value={invokeUrl ?? ""}
              copyable
              copyTooltipText="Copy URL"
              slotProps={{ input: { readOnly: true } }}
            />
            <TextInput
              label="Auth Type"
              value={authLabel}
              slotProps={{ input: { readOnly: true } }}
            />
            <TextInput
              label="Auth Header"
              value={authHeaderExample}
              copyable={authMode !== "none"}
              copyTooltipText="Copy header"
              slotProps={{ input: { readOnly: true } }}
            />
          </Form.Stack>
        </Form.Section>

        {authMode === "oauth" && (
          <Form.Section>
            <Form.Subheader>Auth Endpoint</Form.Subheader>
            <Form.Stack spacing={1.5}>
              {oauthIssuers.length === 0 ? (
                <Alert severity="warning">
                  No identity provider is configured for OAuth on this environment yet.
                </Alert>
              ) : (
                issuerDetails.map((provider) => (
                  <Card key={provider.name} variant="outlined" sx={{ p: 2 }}>
                    <Box display="flex" flexDirection="column" gap={1}>
                      <Typography variant="body2" sx={{ fontWeight: 600 }}>
                        {provider.name}
                      </Typography>
                      {!provider.found && !isLoadingProviders && (
                        <Alert severity="warning" sx={{ py: 0 }}>
                          This identity provider is no longer configured on this environment.
                        </Alert>
                      )}
                      {(
                        [["Issuer", provider.issuer], ["JWKS URI", provider.jwksUri]] as const
                      ).map(([label, value]) => (
                        <TextInput
                          key={label}
                          label={label}
                          value={value ?? (isLoadingProviders ? "Loading…" : "—")}
                          copyable={!!value}
                          slotProps={{ input: { readOnly: true } }}
                        />
                      ))}
                      <Typography variant="caption" color="text.secondary">
                        Get an access token from this provider:
                      </Typography>
                      <CodeBlock
                        code={buildTokenCurl(provider.issuer)}
                        language="bash"
                        fieldId={`token-curl-${provider.name}`}
                        analyticsId="token-curl"
                      />
                    </Box>
                  </Card>
                ))
              )}
            </Form.Stack>
          </Form.Section>
        )}

        <Form.Section>
          <Form.Subheader>Sample cURL</Form.Subheader>
          <Form.Stack spacing={2}>
            <Box display="flex" flexDirection="column" gap={0.5}>
              <Typography variant="body2" color="text.secondary">
                {callAgentCopy}
              </Typography>
              <CodeBlock code={invokeCurl} language="bash" fieldId="invoke-curl" />
            </Box>
          </Form.Stack>
        </Form.Section>
      </DrawerContent>
    </DrawerWrapper>
  );
}

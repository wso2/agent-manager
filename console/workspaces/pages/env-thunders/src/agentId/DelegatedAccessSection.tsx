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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Button, Divider, Stack, Typography } from "@wso2/oxygen-ui";
import { ExternalLink } from "@wso2/oxygen-ui-icons-react";

interface DelegatedAccessSectionProps {
  enabled: boolean;
  consoleUrl?: string;
  orgName: string;
  projectName: string;
  identityId?: string;
  isExternal: boolean;
}

export function DelegatedAccessSection({
  enabled,
  consoleUrl,
  orgName,
  projectName,
  identityId,
  isExternal,
}: DelegatedAccessSectionProps) {
  const base = consoleUrl?.trim();
  if (!enabled || !base || base.startsWith("//")) return null;

  let destination: URL;
  try {
    destination = base.startsWith("/")
      ? new URL(base, window.location.origin)
      : new URL(base);
  } catch {
    return null;
  }
  if (
    !["https:", "http:"].includes(destination.protocol) ||
    (base.startsWith("/") && destination.origin !== window.location.origin) ||
    destination.username ||
    destination.password
  ) {
    return null;
  }

  const routeValues = [orgName, projectName, identityId];
  const href =
    identityId &&
    routeValues.every((value) => value && value !== "." && value !== "..")
      ? `${destination.origin}${destination.pathname.replace(/\/+$/, "")}/organizations/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projectName)}/agents/${encodeURIComponent(identityId)}`
      : undefined;

  return (
    <>
      <Divider />
      <Stack spacing={1.5}>
        <Typography variant="subtitle1" component="h2">
          Delegated Access (OBO)
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Let this agent act on behalf of a signed-in user.
        </Typography>
        <Stack component="ol" spacing={0.5} sx={{ m: 0, pl: 2.5 }}>
          <Typography component="li" variant="body2" color="text.secondary">
            In Identity Platform, open <strong>Advanced</strong> and configure
            delegated access.
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            Deploy the configuration to this environment.
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            {isExternal
              ? "Use this environment’s token endpoint and Agent ID credentials in your external runtime."
              : "Use the Agent ID credentials available to your hosted agent."}
          </Typography>
        </Stack>
        {href ? (
          <Button
            component="a"
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            variant="outlined"
            size="small"
            startIcon={<ExternalLink size={16} />}
            sx={{ alignSelf: "flex-start" }}
          >
            Configure in Identity Platform
          </Button>
        ) : (
          <Typography variant="body2" color="text.secondary">
            Setup is available once the agent has an active Agent ID in this
            environment.
          </Typography>
        )}
      </Stack>
    </>
  );
}

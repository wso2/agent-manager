/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { type ReactNode, useState } from "react";
import { Alert, Box, Button, Stack, Tab, Tabs, Typography } from "@wso2/oxygen-ui";
import { Check, Copy } from "@wso2/oxygen-ui-icons-react";
import {
  useCreateSelfHostedGatewayToken,
  useRevokeSelfHostedGatewayToken,
  useSelfHostedGatewayTokens,
} from "@agent-management-platform/api-client";
import { copyToClipboard } from "@agent-management-platform/shared-component";
import { globalConfig } from "@agent-management-platform/types";
import { useGatewayFeatures } from "@agent-management-platform/feature-management";
import {
  activeTokens,
  cpHostFromUrl,
  dockerCommands,
  helmCommands,
  reconfigureToken,
} from "./selfHostedSetup";

const DEFAULT_GATEWAY_VERSION = "v2026.09.24";
const DEFAULT_CONTROL_PLANE_URL = "http://localhost:9243";
const TOKEN_PLACEHOLDER = "<your-gateway-token>";
const COPY_FEEDBACK_MS = 1500;

interface CommandFieldProps {
  /** What is displayed; carries a placeholder where the token goes. */
  display: string;
  /** What is copied; carries the real token. Defaults to `display`. */
  copyValue?: string;
  copyLabel: string;
}

function CommandField({ display, copyValue, copyLabel }: CommandFieldProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    void copyToClipboard(copyValue ?? display).then((succeeded) => {
      if (!succeeded) return;
      setCopied(true);
      setTimeout(() => setCopied(false), COPY_FEEDBACK_MS);
    });
  };

  return (
    <Box sx={{ border: "1px solid", borderColor: "divider", borderRadius: 1, overflow: "hidden" }}>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          px: 2,
          py: 0.5,
          borderBottom: "1px solid",
          borderColor: "divider",
        }}
      >
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: "monospace" }}>
          bash
        </Typography>
        <Button
          size="small"
          aria-label={`Copy ${copyLabel}`}
          onClick={handleCopy}
          startIcon={copied ? <Check size={14} /> : <Copy size={14} />}
          sx={{ textTransform: "none", color: "text.secondary", minWidth: 0 }}
        >
          {copied ? "Copied" : "Copy"}
        </Button>
      </Box>
      <Box
        component="pre"
        sx={{
          m: 0,
          p: 2,
          fontFamily: "monospace",
          fontSize: "0.875rem",
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
          bgcolor: "action.hover",
        }}
      >
        {display}
      </Box>
    </Box>
  );
}

function Step({ number, title, children }: { number: number; title: string; children: ReactNode }) {
  return (
    <Stack spacing={1}>
      <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
        {number}. {title}
      </Typography>
      {children}
    </Stack>
  );
}

const StepText = ({ children }: { children: ReactNode }) => (
  <Typography variant="body2" color="text.secondary">
    {children}
  </Typography>
);

interface SelfHostedSetupStepsProps {
  orgName: string;
  gatewayId: string;
  /**
   * The token just minted for this gateway. It lives only in memory: the
   * server never returns it again, so without one the steps offer Reconfigure.
   */
  initialToken?: string | null;
}

export function SelfHostedSetupSteps({
  orgName,
  gatewayId,
  initialToken = null,
}: SelfHostedSetupStepsProps) {
  const [tab, setTab] = useState<"docker" | "helm">("docker");
  const [token, setToken] = useState<string | null>(initialToken);
  const [justReconfigured, setJustReconfigured] = useState(false);
  const [reconfiguring, setReconfiguring] = useState(false);
  const [reconfigureError, setReconfigureError] = useState<string | null>(null);
  const { canManageTokens } = useGatewayFeatures();

  const params = { orgName, gatewayId };
  // Read only when reconfiguring, so nothing loads on mount.
  const { refetch: refetchTokens } = useSelfHostedGatewayTokens(params, { enabled: false });
  const { mutateAsync: createToken } = useCreateSelfHostedGatewayToken();
  const { mutateAsync: revokeToken } = useRevokeSelfHostedGatewayToken();

  const version = globalConfig.gatewayVersion || DEFAULT_GATEWAY_VERSION;
  const cpHost = cpHostFromUrl(globalConfig.gatewayControlPlaneUrl || DEFAULT_CONTROL_PLANE_URL);

  const handleReconfigure = async () => {
    setReconfiguring(true);
    setReconfigureError(null);
    try {
      const minted = await reconfigureToken(
        async () => {
          // A failed list must abort before anything is revoked or minted.
          const { data } = await refetchTokens({ throwOnError: true });
          return activeTokens(data?.list ?? []).map((t) => t.id);
        },
        (tokenId) => revokeToken({ ...params, tokenId }),
        async () => (await createToken(params)).token,
      );
      setToken(minted);
      setJustReconfigured(true);
    } catch (e) {
      setReconfigureError(e instanceof Error ? e.message : "Failed to generate a new token.");
    } finally {
      setReconfiguring(false);
    }
  };

  const reconfigurePrompt = canManageTokens ? (
    <Stack spacing={1} alignItems="flex-start">
      <StepText>
        A gateway you already set up keeps working with its previous token. To get a new token and
        a fresh command, click Reconfigure. This revokes the previous tokens.
      </StepText>
      <Button variant="outlined" onClick={handleReconfigure} disabled={reconfiguring}>
        Reconfigure
      </Button>
    </Stack>
  ) : (
    <StepText>The registration token is shown only once, when it is generated.</StepText>
  );

  const tokenNote = (
    <>
      {justReconfigured && (
        <Alert severity="success">
          New configuration generated. Use the command below; the token is shown only once.
        </Alert>
      )}
      {!justReconfigured && token && (
        <Alert severity="info">Copy the command now; the token is not shown again.</Alert>
      )}
    </>
  );

  const docker = dockerCommands(version, cpHost, token ?? TOKEN_PLACEHOLDER);
  const dockerDisplay = dockerCommands(version, cpHost, TOKEN_PLACEHOLDER);
  const helm = helmCommands(version, cpHost, token ?? TOKEN_PLACEHOLDER);
  const helmDisplay = helmCommands(version, cpHost, TOKEN_PLACEHOLDER);

  return (
    <Stack spacing={2}>
      {reconfigureError && <Alert severity="error">{reconfigureError}</Alert>}
      <Tabs value={tab} onChange={(_, value) => setTab(value)}>
        <Tab value="docker" label="Docker" />
        <Tab value="helm" label="Kubernetes (Helm)" />
      </Tabs>

      {tab === "docker" && (
        <Stack spacing={3}>
          <Step number={1} title="Download the gateway">
            <CommandField display={docker.download} copyLabel="download command" />
          </Step>
          <Step number={2} title="Set up the gateway">
            <StepText>
              Run the setup script. It creates the listener certificate, the at-rest encryption key
              and the api-platform.env file.
            </StepText>
            <CommandField display={docker.setup} copyLabel="setup command" />
          </Step>
          <Step number={3} title="Configure the gateway">
            {token ? (
              <>
                {tokenNote}
                <StepText>Add the control plane connection settings to api-platform.env.</StepText>
                <CommandField
                  display={dockerDisplay.configure}
                  copyValue={docker.configure}
                  copyLabel="configure command"
                />
              </>
            ) : (
              reconfigurePrompt
            )}
          </Step>
          <Step number={4} title="Start the gateway">
            <CommandField display={docker.start} copyLabel="start command" />
          </Step>
        </Stack>
      )}

      {tab === "helm" && (
        <Stack spacing={3}>
          <Step number={1} title="Create the encryption key secret">
            <CommandField
              display={helmDisplay.createEncryptionKeySecret}
              copyLabel="encryption key commands"
            />
          </Step>
          <Step number={2} title="Install the chart">
            {token ? (
              <>
                {tokenNote}
                <CommandField
                  display={helmDisplay.install}
                  copyValue={helm.install}
                  copyLabel="helm install command"
                />
              </>
            ) : (
              reconfigurePrompt
            )}
          </Step>
        </Stack>
      )}
    </Stack>
  );
}

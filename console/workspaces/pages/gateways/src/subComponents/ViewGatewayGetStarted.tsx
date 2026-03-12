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

import React, { useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  IconButton,
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { Copy, Computer, Server, Cloud } from "@wso2/oxygen-ui-icons-react";
import { globalConfig } from "@agent-management-platform/types";

const GATEWAY_VERSION = "v1.0";
const GATEWAY_ENV_FILE = `ai-gateway-${GATEWAY_VERSION}/configs/keys.env`;
const GATEWAY_VERSION_HELM = GATEWAY_VERSION.startsWith("v")
  ? GATEWAY_VERSION.slice(1)
  : GATEWAY_VERSION;

const DEFAULT_GATEWAY_CONTROL_PLANE_URL = "http://localhost:9243";

function getGatewayControlPlaneUrl(): string {
  const url =
    globalConfig.gatewayControlPlaneUrl?.trim() ||
    globalConfig.apiBaseUrl?.trim();
  return url || DEFAULT_GATEWAY_CONTROL_PLANE_URL;
}

const getSetupGatewayDisplayCommand = () =>
  `curl -sLO https://github.com/wso2/api-platform/releases/download/ai-gateway/${GATEWAY_VERSION}/ai-gateway-${GATEWAY_VERSION}.zip && \\
unzip ai-gateway-${GATEWAY_VERSION}.zip`;

const getConfigureGatewayDisplayCommand = (registrationToken: string | null) => {
  const controlPlaneHost = new URL(getGatewayControlPlaneUrl());
  const tokenValue = registrationToken || "<your-gateway-token>";
  return `cat > ${GATEWAY_ENV_FILE} << 'ENVFILE'
GATEWAY_CONTROLPLANE_HOST=${controlPlaneHost}
GATEWAY_REGISTRATION_TOKEN=${tokenValue}
ENVFILE`;
};

const getStep3NavigateCommand = () => `cd ai-gateway-${GATEWAY_VERSION}`;

const getStartGatewayDisplayCommand = () =>
  `docker compose --env-file configs/keys.env up`;

const getK8sCustomHelmDisplayCommand = (registrationToken: string | null) => {
  const controlPlaneHost = new URL(getGatewayControlPlaneUrl()).hostname;
  const tokenValue = registrationToken || "your-gateway-token";
  return `helm install gateway oci://ghcr.io/wso2/api-platform/helm-charts/gateway --version ${GATEWAY_VERSION_HELM} \\
  --set gateway.controller.controlPlane.host="${controlPlaneHost}" \\
  --set gateway.controller.controlPlane.port=443 \\
  --set gateway.controller.controlPlane.token.value="${tokenValue}" \\
  --set gateway.config.analytics.enabled=true`;
};

const commandTextFieldSx = {
  "& .MuiInputBase-input": {
    fontFamily: "monospace",
    fontSize: "0.875rem",
  },
};

type TabPanelProps = {
  value: number;
  index: number;
  children: React.ReactNode;
};

function TabPanel({ value, index, children }: TabPanelProps) {
  return (
    <Box role="tabpanel" hidden={value !== index} sx={{ pt: 2 }}>
      {value === index ? children : null}
    </Box>
  );
}

interface CommandFieldProps {
  value: string;
  multiline?: boolean;
  minRows?: number;
  onCopy: () => void;
  copyLabel: string;
}

function CommandField({
  value,
  multiline,
  minRows = 1,
  onCopy,
  copyLabel,
}: CommandFieldProps) {
  return (
    <TextField
      fullWidth
      multiline={multiline}
      minRows={minRows}
      value={value}
      slotProps={{
        input: {
          readOnly: true,
          endAdornment: (
            <IconButton size="small" onClick={onCopy} aria-label={`Copy ${copyLabel}`}>
              <Copy size={16} />
            </IconButton>
          ),
        },
      }}
      sx={commandTextFieldSx}
    />
  );
}

interface ViewGatewayGetStartedProps {
  isConfigured: boolean;
  registrationToken: string | null;
  hasJustRegeneratedToken: boolean;
  onRegenerateToken: () => void;
  isRegeneratingToken: boolean;
  onCopy: (text: string, label: string) => void;
}

const TAB_PARAM = "tab";
const VALID_TABS = [0, 1, 2, 3] as const;

export function ViewGatewayGetStarted({
  isConfigured,
  registrationToken,
  hasJustRegeneratedToken,
  onRegenerateToken,
  isRegeneratingToken,
  onCopy,
}: ViewGatewayGetStartedProps) {
  const [searchParams, setSearchParams] = useSearchParams();
  const tabFromUrl = searchParams.get(TAB_PARAM);
  const parsedTab = tabFromUrl != null ? parseInt(tabFromUrl, 10) : 0;
  const tabIndex = VALID_TABS.includes(parsedTab as (typeof VALID_TABS)[number])
    ? parsedTab
    : 0;

  const handleTabChange = useCallback(
    (_: React.SyntheticEvent, newValue: number) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set(TAB_PARAM, String(newValue));
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const handleCopy = useCallback(
    (text: string, label: string) => {
      navigator.clipboard.writeText(text);
      onCopy(text, label);
    },
    [onCopy],
  );

  const renderStep2 = () => {
    if (registrationToken) {
      return (
        <>
          {hasJustRegeneratedToken && (
            <Alert severity="success" sx={{ mb: 2 }}>
              Successfully generated new configurations. Use the updated command
              below to reconfigure your gateway.
            </Alert>
          )}
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Run this command to create {GATEWAY_ENV_FILE} with the required
            environment variables:
          </Typography>
          <CommandField
            value={getConfigureGatewayDisplayCommand(registrationToken)}
            multiline
            minRows={4}
            onCopy={() =>
              handleCopy(
                getConfigureGatewayDisplayCommand(registrationToken),
                "Configure command",
              )
            }
            copyLabel="Configure command"
          />
        </>
      );
    }
    return (
      <>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {isConfigured
            ? "Regenerate a registration token to configure your gateway."
            : "Generate a registration token to configure your gateway for the first time."}
        </Typography>
        <Button
          variant="outlined"
          color="primary"
          onClick={onRegenerateToken}
          disabled={isRegeneratingToken}
        >
          {isConfigured ? "Reconfigure" : "Configure"}
        </Button>
      </>
    );
  };

  const renderQuickStartSteps = () => (
    <Stack spacing={3}>
      <Box>
        <Typography variant="h6" sx={{ mb: 2 }}>
          Prerequisites
        </Typography>
        <Stack component="ul" spacing={0.5} sx={{ pl: 3 }}>
          <Typography component="li" variant="body2" color="text.secondary">
            cURL installed
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            unzip installed
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            Docker installed and running
          </Typography>
        </Stack>
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 1: Download the Gateway
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Run this command in your terminal to download the gateway:
        </Typography>
        <CommandField
          value={getSetupGatewayDisplayCommand()}
          multiline
          minRows={2}
          onCopy={() =>
            handleCopy(getSetupGatewayDisplayCommand(), "Download command")
          }
          copyLabel="Download command"
        />
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 2: Configure the Gateway
        </Typography>
        {renderStep2()}
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 3: Start the Gateway
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          1. Navigate to the gateway folder.
        </Typography>
        <CommandField
          value={getStep3NavigateCommand()}
          onCopy={() =>
            handleCopy(getStep3NavigateCommand(), "Navigate command")
          }
          copyLabel="Navigate command"
        />
        <Typography variant="body2" color="text.secondary" sx={{ my: 2 }}>
          2. Run this command to start the gateway using the configs/keys.env
          file created in Step 2:
        </Typography>
        <CommandField
          value={getStartGatewayDisplayCommand()}
          onCopy={() =>
            handleCopy(getStartGatewayDisplayCommand(), "Start command")
          }
          copyLabel="Start command"
        />
      </Box>
    </Stack>
  );

  const renderVMTab = () => (
    <Stack spacing={3}>
      <Box>
        <Typography variant="h6" sx={{ mb: 2 }}>
          Prerequisites
        </Typography>
        <Stack component="ul" spacing={0.5} sx={{ pl: 3 }}>
          <Typography component="li" variant="body2" color="text.secondary">
            cURL installed
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            unzip installed
          </Typography>
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1.5, mb: 1 }}>
          A Docker-compatible container runtime such as:
        </Typography>
        <Stack component="ul" spacing={0.5} sx={{ pl: 3 }}>
          <Typography component="li" variant="body2" color="text.secondary">
            Docker Desktop (Windows / macOS)
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            Rancher Desktop (Windows / macOS)
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            Colima (macOS)
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            Docker Engine + Compose plugin (Linux)
          </Typography>
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 1.5, mb: 1.5 }}>
          Ensure docker and docker compose commands are available.
        </Typography>
        <CommandField
          value="docker --version\ndocker compose version"
          multiline
          minRows={2}
          onCopy={() =>
            handleCopy(
              "docker --version\ndocker compose version",
              "Prerequisites command",
            )
          }
          copyLabel="Prerequisites command"
        />
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 1: Download the Gateway
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Run this command in your terminal to download the gateway:
        </Typography>
        <CommandField
          value={getSetupGatewayDisplayCommand()}
          multiline
          minRows={2}
          onCopy={() =>
            handleCopy(getSetupGatewayDisplayCommand(), "Download command")
          }
          copyLabel="Download command"
        />
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 2: Configure the Gateway
        </Typography>
        {renderStep2()}
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 3: Start the Gateway
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          1. Navigate to the gateway folder.
        </Typography>
        <CommandField
          value={getStep3NavigateCommand()}
          onCopy={() =>
            handleCopy(getStep3NavigateCommand(), "Navigate command")
          }
          copyLabel="Navigate command"
        />
        <Typography variant="body2" color="text.secondary" sx={{ my: 2 }}>
          2. Run this command to start the gateway using the configs/keys.env
          file created in Step 2:
        </Typography>
        <CommandField
          value={getStartGatewayDisplayCommand()}
          onCopy={() =>
            handleCopy(getStartGatewayDisplayCommand(), "Start command")
          }
          copyLabel="Start command"
        />
      </Box>
    </Stack>
  );

  const renderDockerTab = () => (
    <Stack spacing={3}>
      <Box>
        <Typography variant="h6" sx={{ mb: 2 }}>
          Prerequisites
        </Typography>
        <Stack component="ul" spacing={0.5} sx={{ pl: 3 }}>
          <Typography component="li" variant="body2" color="text.secondary">
            cURL installed
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            unzip installed
          </Typography>
        </Stack>
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 1: Download the Gateway
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Run this command in your terminal to download the gateway:
        </Typography>
        <CommandField
          value={getSetupGatewayDisplayCommand()}
          multiline
          minRows={2}
          onCopy={() =>
            handleCopy(getSetupGatewayDisplayCommand(), "Download command")
          }
          copyLabel="Download command"
        />
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 2: Configure the Gateway
        </Typography>
        {renderStep2()}
      </Box>

      <Box>
        <Typography variant="h6" sx={{ mb: 1 }} color="warning.main">
          Step 3: Start the Gateway
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          1. Navigate to the gateway folder.
        </Typography>
        <CommandField
          value={getStep3NavigateCommand()}
          onCopy={() =>
            handleCopy(getStep3NavigateCommand(), "Navigate command")
          }
          copyLabel="Navigate command"
        />
        <Typography variant="body2" color="text.secondary" sx={{ my: 2 }}>
          2. Run this command to start the gateway using the configs/keys.env
          file created in Step 2:
        </Typography>
        <CommandField
          value={getStartGatewayDisplayCommand()}
          onCopy={() =>
            handleCopy(getStartGatewayDisplayCommand(), "Start command")
          }
          copyLabel="Start command"
        />
      </Box>
    </Stack>
  );

  const renderKubernetesTab = () => (
    <Stack spacing={3}>
      <Box>
        <Typography variant="h6" sx={{ mb: 1 }}>
          Prerequisites
        </Typography>
        <Stack component="ul" spacing={0.5} sx={{ pl: 3 }}>
          <Typography component="li" variant="body2" color="text.secondary">
            cURL installed
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            unzip installed
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            Kubernetes 1.32+
          </Typography>
          <Typography component="li" variant="body2" color="text.secondary">
            Helm 3.18+
          </Typography>
        </Stack>
      </Box>

      <Box>
        {registrationToken ? (
          hasJustRegeneratedToken && (
            <Alert severity="success" sx={{ mb: 2 }}>
              Successfully generated new configurations. Use the updated command
              below to install the gateway chart.
            </Alert>
          )
        ) : (
          <>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              {isConfigured
                ? "Regenerate a registration token to configure your gateway."
                : "Generate a registration token to configure your gateway for the first time."}
            </Typography>
            <Button
              variant="outlined"
              color="primary"
              onClick={onRegenerateToken}
              disabled={isRegeneratingToken}
            >
              {isConfigured ? "Reconfigure" : "Configure"}
            </Button>
          </>
        )}
      </Box>

      <Box>
        <Typography variant="h6" color="warning.main">
          Installing the Chart
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Run this command to install the gateway chart with control plane
          configurations:
        </Typography>
        <CommandField
          value={getK8sCustomHelmDisplayCommand(registrationToken)}
          multiline
          minRows={2}
          onCopy={() =>
            handleCopy(
              getK8sCustomHelmDisplayCommand(registrationToken),
              "Helm install command",
            )
          }
          copyLabel="Helm install command"
        />
      </Box>
    </Stack>
  );

  return (
    <Card variant="outlined">
      <CardContent sx={{ p: 3 }}>
        <Typography variant="h5" sx={{ mb: 3 }}>
          Get Started
        </Typography>

        <Box sx={{ borderBottom: 1, borderColor: "divider", mb: 2 }}>
          <Tabs value={tabIndex} onChange={handleTabChange}>
            <Tab label="Quick Start" />
            <Tab label="Virtual Machine" icon={<Computer size={20} />} iconPosition="start" />
            <Tab label="Docker" icon={<Server size={20} />} iconPosition="start" />
            <Tab label="Kubernetes" icon={<Cloud size={20} />} iconPosition="start" />
          </Tabs>
        </Box>

        <TabPanel value={tabIndex} index={0}>
          {renderQuickStartSteps()}
        </TabPanel>
        <TabPanel value={tabIndex} index={1}>
          {renderVMTab()}
        </TabPanel>
        <TabPanel value={tabIndex} index={2}>
          {renderDockerTab()}
        </TabPanel>
        <TabPanel value={tabIndex} index={3}>
          {renderKubernetesTab()}
        </TabPanel>
      </CardContent>
    </Card>
  );
}

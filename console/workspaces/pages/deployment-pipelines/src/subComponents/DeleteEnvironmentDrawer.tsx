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

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  IconButton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Copy, Eye, EyeOff, Trash } from "@wso2/oxygen-ui-icons-react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  getAgentManagerUrl,
  getRawScriptUrl,
} from "@agent-management-platform/shared-component";
import type { Environment } from "@agent-management-platform/types";

const TOKEN_MASK = "•••••••••••••••";

interface DeleteEnvironmentDrawerProps {
  open: boolean;
  onClose: () => void;
  environment: Environment;
}

function buildScript(name: string, token: string): string {
  // The remove-environment.sh script deprovisions the environment's ThunderID
  // instance, uninstalls its API Platform Gateway helm release, and then deletes
  // the Environment via the Agent Manager API. Pass the version-matched
  // THUNDER_SCRIPT_URL so the chained call fetches the script from the same git
  // ref as this one. ORG_NAME is left to the script's default so release names
  // match what add-environment.sh installed.
  const lines = [
    `curl -fsSL ${getRawScriptUrl("remove-environment.sh")} \\`,
    `  | ENV_NAME=${name || "<env-name>"} \\`,
    `    AGENT_MANAGER_TOKEN=${token} \\`,
    `    AGENT_MANAGER_URL=${getAgentManagerUrl()} \\`,
    `    THUNDER_SCRIPT_URL=${getRawScriptUrl("remove-environment-thunder.sh")} \\`,
    "    bash",
  ];
  return lines.join("\n");
}

export function DeleteEnvironmentDrawer({
  open,
  onClose,
  environment,
}: DeleteEnvironmentDrawerProps) {
  const [showToken, setShowToken] = useState(false);
  const [resolvedToken, setResolvedToken] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const { getToken } = useAuthHooks();

  useEffect(() => {
    if (open) {
      setShowToken(false);
      setResolvedToken(null);
      setCopied(false);
    }
  }, [open]);

  const handleToggleToken = useCallback(async () => {
    if (showToken) {
      setShowToken(false);
      setResolvedToken(null);
    } else {
      try {
        const token = await getToken();
        setResolvedToken(token);
        setShowToken(true);
      } catch {
        // silently fail
      }
    }
  }, [showToken, getToken]);

  const handleCopy = useCallback(async () => {
    try {
      const token = resolvedToken ?? (await getToken());
      const script = buildScript(environment.name, token);
      await navigator.clipboard.writeText(script);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // silently fail
    }
  }, [resolvedToken, getToken, environment.name]);

  const displayScript = useMemo(
    () => buildScript(environment.name, showToken && resolvedToken ? resolvedToken : TOKEN_MASK),
    [environment.name, showToken, resolvedToken],
  );

  return (
    <DrawerWrapper open={open} onClose={onClose}>
      <DrawerHeader icon={<Trash size={24} />} title="Delete Environment" onClose={onClose} />
      <DrawerContent>
        <Stack spacing={3}>
          <Typography variant="body2" color="text.secondary">
            Environments are removed by a script that deprovisions the environment&apos;s dedicated
            identity (Thunder) instance, uninstalls its API Platform Gateway via Helm, and then
            deletes the environment in Agent Manager. Copy and run the command below in a terminal
            with <code>kubectl</code> and <code>helm</code> configured against your cluster.
          </Typography>

          <Alert severity="warning">
            This permanently deletes <strong>{environment.displayName ?? environment.name}</strong>{" "}
            and uninstalls its gateway. Agents deployed to this environment will no longer be
            reachable. This action cannot be undone.
          </Alert>

          <Stack spacing={1}>
            <Box
              sx={{
                position: "relative",
                bgcolor: "grey.900",
                borderRadius: 1,
                p: 2,
                pr: 8,
                fontFamily: "monospace",
                fontSize: "0.8125rem",
                color: "grey.100",
                whiteSpace: "pre",
                overflowX: "auto",
              }}
            >
              <Box sx={{ position: "absolute", top: 6, right: 6, display: "flex", gap: 0.5 }}>
                <Tooltip title={showToken ? "Hide token" : "Show token"}>
                  <IconButton
                    size="small"
                    onClick={handleToggleToken}
                    sx={{ color: "grey.400" }}
                  >
                    {showToken ? <EyeOff size={16} /> : <Eye size={16} />}
                  </IconButton>
                </Tooltip>
                <Tooltip title={copied ? "Copied!" : "Copy"}>
                  <IconButton
                    size="small"
                    onClick={handleCopy}
                    sx={{ color: copied ? "success.light" : "grey.400" }}
                  >
                    <Copy size={16} />
                  </IconButton>
                </Tooltip>
              </Box>
              {displayScript}
            </Box>
            <Typography variant="caption" color="text.secondary">
              Your access token will be substituted when you copy.
            </Typography>
          </Stack>

          <Alert severity="info">
            Once the script completes, the environment will disappear from the list. The script is
            idempotent — safe to re-run.
          </Alert>

          <Box display="flex" justifyContent="flex-end">
            <Button variant="outlined" color="inherit" onClick={onClose}>
              Close
            </Button>
          </Box>
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

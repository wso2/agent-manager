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

import { useState } from "react";
import {
  Alert,
  Button,
  Card,
  Chip,
  IconButton,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Copy, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import {
  useCreateSelfHostedGatewayToken,
  useRevokeSelfHostedGatewayToken,
  useSelfHostedGatewayTokens,
} from "@agent-management-platform/api-client";
import {
  getErrorMessage,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";

const MAX_ACTIVE_TOKENS = 2;

interface SelfHostedGatewayTokensCardProps {
  orgName: string;
  gatewayId: string;
}

export function SelfHostedGatewayTokensCard({
  orgName,
  gatewayId,
}: SelfHostedGatewayTokensCardProps) {
  const params = { orgName, gatewayId };
  const { data, isLoading, error } = useSelfHostedGatewayTokens(params);
  const { mutateAsync: createToken, isPending: creating } = useCreateSelfHostedGatewayToken();
  const { mutateAsync: revokeToken } = useRevokeSelfHostedGatewayToken();
  const { addConfirmation } = useConfirmationDialog();
  // Shown once, straight after minting; never stored or refetched.
  const [newToken, setNewToken] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const tokens = data?.list ?? [];
  const activeCount = tokens.filter((t) => t.status === "active").length;

  const handleCreate = async () => {
    setActionError(null);
    try {
      setNewToken((await createToken(params)).token);
    } catch (e) {
      setActionError(getErrorMessage(e) || "Failed to create a token.");
    }
  };

  const handleRevoke = (tokenId: string) =>
    addConfirmation({
      analytics: { entity: "gateway-token", action: "revoke" },
      title: "Revoke token",
      description:
        "A gateway using this token will no longer be able to connect to the control plane.",
      confirmButtonText: "Revoke",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: async () => {
        await revokeToken({ ...params, tokenId });
      },
    });

  return (
    <Card variant="outlined" sx={{ p: 3 }}>
      <Stack spacing={2}>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography variant="h6">Registration tokens</Typography>
          <Tooltip
            title={
              activeCount >= MAX_ACTIVE_TOKENS
                ? `A gateway can have at most ${MAX_ACTIVE_TOKENS} active tokens. Revoke one first.`
                : ""
            }
          >
            <span>
              <Button
                size="small"
                variant="outlined"
                startIcon={<Plus size={16} />}
                onClick={handleCreate}
                disabled={creating || activeCount >= MAX_ACTIVE_TOKENS}
              >
                New token
              </Button>
            </span>
          </Tooltip>
        </Stack>

        {actionError && <Alert severity="error">{actionError}</Alert>}
        {newToken && (
          <Alert
            severity="success"
            onClose={() => setNewToken(null)}
            action={
              <Button
                size="small"
                startIcon={<Copy size={14} />}
                onClick={() => void navigator.clipboard.writeText(newToken)}
              >
                Copy
              </Button>
            }
          >
            Token created. Copy it now; it is not shown again.
          </Alert>
        )}

        {isLoading && <Skeleton variant="rounded" height={44} />}
        {error ? <Alert severity="error">Failed to load tokens.</Alert> : null}
        {!isLoading && !error && tokens.length === 0 && (
          <Typography variant="body2" color="text.secondary">
            No tokens yet.
          </Typography>
        )}
        {tokens.map((token) => (
          <Stack
            key={token.id}
            direction="row"
            alignItems="center"
            spacing={2}
            sx={{ px: 2, py: 1, border: "1px solid", borderColor: "divider", borderRadius: 1 }}
          >
            <Typography variant="body2" sx={{ fontFamily: "monospace", flex: 1 }}>
              {token.id}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              Created {formatDistanceToNow(new Date(token.createdAt), { addSuffix: true })}
            </Typography>
            <Chip
              label={token.status === "active" ? "Active" : "Revoked"}
              size="small"
              variant="outlined"
              color={token.status === "active" ? "success" : "default"}
            />
            {token.status === "active" && (
              <Tooltip title="Revoke">
                <IconButton
                  size="small"
                  color="error"
                  aria-label="Revoke token"
                  onClick={() => handleRevoke(token.id)}
                >
                  <Trash size={16} />
                </IconButton>
              </Tooltip>
            )}
          </Stack>
        ))}
      </Stack>
    </Card>
  );
}

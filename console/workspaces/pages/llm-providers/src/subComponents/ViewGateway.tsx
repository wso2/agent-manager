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

import React, { useState } from "react";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { AlertTriangle, Pencil } from "@wso2/oxygen-ui-icons-react";
import { formatDistanceToNow } from "date-fns";
import { generatePath, useParams } from "react-router-dom";
import { useGetGateway } from "@agent-management-platform/api-client";
import { PageLayout } from "@agent-management-platform/views";
import { absoluteRouteMap } from "@agent-management-platform/types";
import type { GatewayResponse } from "@agent-management-platform/types";
import { EditGatewayDrawer } from "./EditGatewayDrawer";

export const ViewGateway: React.FC = () => {
  const { gatewayId, orgId } = useParams<{
    gatewayId: string;
    orgId: string;
  }>();
  const [editDrawerOpen, setEditDrawerOpen] = useState(false);

  const {
    data: gateway,
    isLoading,
    error,
    refetch,
  } = useGetGateway({
    orgName: orgId,
    gatewayId,
  });

  const displayName = gateway?.displayName ?? gateway?.name ?? gatewayId ?? "";
  const isActive =
    gateway?.status === "ACTIVE" ||
    (gateway as { isActive?: boolean } | undefined)?.isActive;

  return (
    <>
      <PageLayout
        title={displayName}
        backHref={generatePath(
          absoluteRouteMap.children.org.children.gateways.path,
          { orgId: orgId ?? "" },
        )}
        backLabel="Back to AI Gateways"
        disableIcon
        isLoading={isLoading}
        actions={
          gateway ? (
            <Button
              variant="contained"
              size="small"
              startIcon={<Pencil size={16} />}
              onClick={() => setEditDrawerOpen(true)}
            >
              Edit
            </Button>
          ) : undefined
        }
        titleTail={
          gateway ? (
            <Stack direction="row" spacing={1} alignItems="center" sx={{ ml: 1 }}>
              <Chip
                label={isActive ? "Active" : "Inactive"}
                size="small"
                variant="outlined"
                color={isActive ? "success" : "default"}
              />
            </Stack>
          ) : undefined
        }
      >
        {error && (
          <Alert
            severity="error"
            icon={<AlertTriangle size={18} />}
            sx={{ mb: 2 }}
          >
            Failed to load gateway.{" "}
            {error instanceof Error ? error.message : "Please try again."}
          </Alert>
        )}

        {gateway && !error && (
          <Stack spacing={3}>
            <Card variant="outlined">
              <CardContent>
                <Typography variant="subtitle1" fontWeight={600} gutterBottom>
                  Overview
                </Typography>
                <Stack spacing={1.5}>
                  <Box>
                    <Typography variant="caption" color="text.secondary">
                      Name
                    </Typography>
                    <Typography variant="body2">
                      {gateway.displayName || gateway.name}
                    </Typography>
                  </Box>
                  <Box>
                    <Typography variant="caption" color="text.secondary">
                      Identifier
                    </Typography>
                    <Typography variant="body2">{gateway.name}</Typography>
                  </Box>
                  <Box>
                    <Typography variant="caption" color="text.secondary">
                      Status
                    </Typography>
                    <Typography variant="body2">{gateway.status}</Typography>
                  </Box>
                  <Box>
                    <Typography variant="caption" color="text.secondary">
                      Type
                    </Typography>
                    <Typography variant="body2">{gateway.gatewayType}</Typography>
                  </Box>
                  <Box>
                    <Typography variant="caption" color="text.secondary">
                      Created
                    </Typography>
                    <Typography variant="body2">
                      {gateway.createdAt
                        ? formatDistanceToNow(new Date(gateway.createdAt), {
                            addSuffix: true,
                          })
                        : "—"}
                    </Typography>
                  </Box>
                  <Box>
                    <Typography variant="caption" color="text.secondary">
                      Last Updated
                    </Typography>
                    <Typography variant="body2">
                      {gateway.updatedAt
                        ? formatDistanceToNow(new Date(gateway.updatedAt), {
                            addSuffix: true,
                          })
                        : "—"}
                    </Typography>
                  </Box>
                </Stack>
              </CardContent>
            </Card>

            <Card variant="outlined">
              <CardContent>
                <Typography variant="subtitle1" fontWeight={600} gutterBottom>
                  Configuration
                </Typography>
                <Stack spacing={1.5}>
                  <Box>
                    <Typography variant="caption" color="text.secondary">
                      Virtual Host
                    </Typography>
                    <Typography variant="body2">{gateway.vhost}</Typography>
                  </Box>
                  {gateway.region && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">
                        Region
                      </Typography>
                      <Typography variant="body2">{gateway.region}</Typography>
                    </Box>
                  )}
                  <Box>
                    <Typography variant="caption" color="text.secondary">
                      Critical
                    </Typography>
                    <Typography variant="body2">
                      {gateway.isCritical ? "Yes" : "No"}
                    </Typography>
                  </Box>
                  {gateway.environments && gateway.environments.length > 0 && (
                    <Box>
                      <Typography variant="caption" color="text.secondary">
                        Environments
                      </Typography>
                      <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                        {gateway.environments.map((env) => (
                          <Chip
                            key={env.id}
                            label={env.displayName || env.name}
                            size="small"
                            variant="outlined"
                          />
                        ))}
                      </Stack>
                    </Box>
                  )}
                </Stack>
              </CardContent>
            </Card>
          </Stack>
        )}
      </PageLayout>

      {gateway && (
        <EditGatewayDrawer
          open={editDrawerOpen}
          onClose={() => setEditDrawerOpen(false)}
          gateway={gateway}
          orgId={orgId ?? ""}
          onSuccess={() => {
            refetch();
            setEditDrawerOpen(false);
          }}
        />
      )}
    </>
  );
};

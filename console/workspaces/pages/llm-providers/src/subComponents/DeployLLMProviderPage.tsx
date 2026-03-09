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

import { useCallback, useMemo, useState } from "react";
import {
  useCreateLLMDeployment,
  useGetLLMProvider,
  useListGateways,
  useListLLMDeployments,
} from "@agent-management-platform/api-client";
import {
  type DeployLLMProviderRequest,
  absoluteRouteMap,
} from "@agent-management-platform/types";
import { PageLayout } from "@agent-management-platform/views";
import {
  Alert,
  Button,
  Card,
  CardContent,
  Collapse,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Rocket, ServerCog } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import { z } from "zod";

const DeployPayloadSchema = z.object({
  name: z.string().min(1, "Deployment name is required"),
  gatewayId: z.string().min(1, "Gateway is required"),
  base: z.string().optional(),
});

export function DeployLLMProviderPage() {
  const { providerId, orgId } = useParams<{
    providerId: string;
    orgId: string;
  }>();

  const { data: providerData, isLoading: isLoadingProvider, error: providerError } =
    useGetLLMProvider({ orgName: orgId, providerId });

  const {
    data: gatewaysData,
    isLoading: isLoadingGateways,
    error: gatewaysError,
  } = useListGateways({ orgName: orgId });

  const { data: deploymentsData } = useListLLMDeployments({
    orgName: orgId,
    providerId,
  });

  const { mutateAsync: deployProvider, isPending: isDeploying } =
    useCreateLLMDeployment();

  const [status, setStatus] = useState<{
    message: string;
    severity: "success" | "error";
  } | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [deployingGatewayId, setDeployingGatewayId] = useState<string | null>(
    null,
  );

  const gateways = useMemo(() => gatewaysData?.gateways ?? [], [gatewaysData]);
  const deployments = useMemo(() => deploymentsData ?? [], [deploymentsData]);

  const filteredGateways = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    if (!q) return gateways;
    return gateways.filter(
      (g) =>
        (g.displayName || g.name).toLowerCase().includes(q) ||
        g.name.toLowerCase().includes(q) ||
        g.vhost?.toLowerCase().includes(q),
    );
  }, [gateways, searchQuery]);

  const isDeployedOnGateway = useCallback(
    (gatewayId: string) =>
      deployments.some(
        (d) => {
          const dep = d as { gatewayId?: string; status?: string };
          return dep.gatewayId === gatewayId && dep.status === "DEPLOYED";
        },
      ),
    [deployments],
  );

  const handleDeploy = useCallback(
    async (gatewayId: string, gatewayName: string) => {
      if (!orgId || !providerId) {
        setStatus({ message: "Provider ID is required", severity: "error" });
        return;
      }

      const name = `${providerData?.name ?? providerId}-${gatewayName}-${Date.now()}`.replace(
        /[^a-zA-Z0-9-_]/g,
        "-",
      );
      const payload = { name, gatewayId, base: "current" };

      const result = DeployPayloadSchema.safeParse(payload);
      if (!result.success) {
        const first = result.error.issues[0];
        setStatus({
          message: first?.message ?? "Validation failed",
          severity: "error",
        });
        return;
      }

      setDeployingGatewayId(gatewayId);
      setStatus(null);

      try {
        await deployProvider({
          params: { orgName: orgId, providerId },
          body: result.data as DeployLLMProviderRequest,
        });
        setStatus({
          message: "Deployed successfully",
          severity: "success",
        });
      } catch (err) {
        setStatus({
          message:
            (err as Error)?.message ?? "Failed to deploy.",
          severity: "error",
        });
      } finally {
        setDeployingGatewayId(null);
      }
    },
    [orgId, providerId, providerData?.name, deployProvider],
  );

  const providerDetailPath = generatePath(
    absoluteRouteMap.children.org.children.llmProviders.children.view.path,
    { orgId: orgId ?? "", providerId: providerId ?? "" },
  );


  if (isLoadingProvider) {
    return (
      <PageLayout title="Deploy to Gateway" disableIcon>
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={40} />
          <Skeleton variant="rounded" height={200} />
        </Stack>
      </PageLayout>
    );
  }

  if (providerError || !providerData) {
    return (
      <PageLayout title="Deploy to Gateway" disableIcon>
        <Alert severity="error">
          {providerError instanceof Error
            ? providerError.message
            : "Failed to load provider."}
        </Alert>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title="Deploy to Gateway"
      description="Deploy Service Provider to your Gateways"
      backHref={providerDetailPath}
      backLabel="Back to Service Provider"
      disableIcon
    >
      <Stack spacing={3}>
        <Collapse in={!!status} timeout={300}>
          {status && (
            <Alert
              severity={status.severity}
              onClose={() => setStatus(null)}
              sx={{ width: "100%" }}
            >
              {status.message}
            </Alert>
          )}
        </Collapse>

        {gatewaysError && (
          <Alert severity="error" sx={{ width: "100%" }}>
            Failed to load gateways. {(gatewaysError as Error)?.message}
          </Alert>
        )}

        {isLoadingGateways ? (
          <Stack spacing={1.5}>
            <Skeleton variant="text" width="40%" height={24} />
            <Skeleton variant="rounded" height={120} />
            <Skeleton variant="rounded" height={120} />
          </Stack>
        ) : gateways.length === 0 ? (
          <ListingTable.Container>
            <ListingTable.EmptyState
              illustration={<ServerCog size={64} />}
              title="No gateways available"
              description="Add gateway to deploy service provider."
            />
          </ListingTable.Container>
        ) : (
          <Stack spacing={2}>
            <SearchBar
              placeholder="Search gateways"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              size="small"
              fullWidth
            />
            <Stack spacing={1.5}>
              {filteredGateways.map((gateway) => {
                const deployed = isDeployedOnGateway(gateway.uuid);
                const isDeployingThis =
                  deployingGatewayId === gateway.uuid && isDeploying;

                return (
                  <Card key={gateway.uuid} variant="outlined">
                    <CardContent>
                      <Stack
                        direction="row"
                        justifyContent="space-between"
                        alignItems="center"
                        flexWrap="wrap"
                        gap={1}
                      >
                        <Stack spacing={0.25}>
                          <Typography variant="body2" fontWeight={500}>
                            {gateway.displayName || gateway.name}
                          </Typography>
                          {gateway.vhost && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                              sx={{ fontFamily: "monospace" }}
                            >
                              {gateway.vhost}
                            </Typography>
                          )}
                          {gateway.status && (
                            <Typography variant="caption" color="text.secondary">
                              {gateway.status}
                            </Typography>
                          )}
                        </Stack>
                        <Button
                          variant="contained"
                          size="small"
                          startIcon={<Rocket size={16} />}
                          onClick={() =>
                            handleDeploy(
                              gateway.uuid,
                              gateway.displayName || gateway.name,
                            )
                          }
                          disabled={deployed || isDeployingThis}
                        >
                          {isDeployingThis
                            ? "Deploying..."
                            : deployed
                              ? "Deployed"
                              : "Deploy"}
                        </Button>
                      </Stack>
                    </CardContent>
                  </Card>
                );
              })}
              {filteredGateways.length === 0 && searchQuery && (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<ServerCog size={64} />}
                    title="No gateways match your search"
                    description="Try a different keyword or clear the search filter."
                  />
                </ListingTable.Container>
              )}
            </Stack>
          </Stack>
        )}
      </Stack>
    </PageLayout>
  );
}

export default DeployLLMProviderPage;

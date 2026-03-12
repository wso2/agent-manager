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

import { type ChangeEvent, useCallback, useEffect, useMemo, useState } from "react";
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
  Avatar,
  Box,
  Button,
  Chip,
  Collapse,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Rocket, ServerCog } from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import { z } from "zod";

const DeployPayloadSchema = z.object({
  name: z.string().min(1, "Deployment name is required"),
  gatewayId: z.string().min(1, "Gateway is required"),
  base: z.string().optional(),
});

type LLMProviderDeployment = {
  gatewayId?: string;
  status?: string;
  [key: string]: unknown;
};

export function DeployLLMProviderPage() {
  const { providerId, orgId } = useParams<{
    providerId: string;
    orgId: string;
  }>();

  const {
    data: providerData,
    isLoading: isLoadingProvider,
    error: providerError,
  } = useGetLLMProvider({ orgName: orgId, providerId });

  const {
    data: gatewaysData,
    isLoading: isLoadingGateways,
    error: gatewaysError,
  } = useListGateways({ orgName: orgId }, { type: "AI", status: "ACTIVE" });

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
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);
  const [deployingGatewayId, setDeployingGatewayId] = useState<string | null>(
    null,
  );

  const gateways = useMemo(() => gatewaysData?.gateways ?? [], [gatewaysData]);
  const deployments = useMemo(
    () =>
      (deploymentsData ?? []) as unknown as LLMProviderDeployment[],
    [deploymentsData],
  );

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

  useEffect(() => {
    if (page !== 0 && page * rowsPerPage >= filteredGateways.length) {
      setPage(0);
    }
  }, [filteredGateways.length, page, rowsPerPage]);

  const isDeployedOnGateway = useCallback(
    (gatewayId: string) =>
      deployments.some(
        (d) => d.gatewayId === gatewayId && d.status === "DEPLOYED",
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

  const isLoading = isLoadingProvider || isLoadingGateways;

  const toolbar = (
    <Stack direction="row" spacing={1} alignItems="center">
      <Box flexGrow={1}>
        <SearchBar
          placeholder="Search gateways..."
          size="small"
          fullWidth
          value={searchQuery}
          onChange={(e: ChangeEvent<HTMLInputElement>) =>
            setSearchQuery(e.target.value)
          }
          disabled={isLoading}
        />
      </Box>
    </Stack>
  );

  if (providerError || (!isLoadingProvider && !providerData)) {
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

  if (isLoadingProvider) {
    return (
      <PageLayout
        title="Deploy to Gateway"
        backHref={providerDetailPath}
        backLabel="Back to Service Provider"
        disableIcon
        isLoading
      >
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={40} />
          <Skeleton variant="rounded" height={200} />
        </Stack>
      </PageLayout>
    );
  }

  return (
    <PageLayout
      title="Deploy to Gateway"
      backHref={providerDetailPath}
      backLabel="Back to Service Provider"
      disableIcon
      isLoading={isLoadingGateways}
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
            Failed to load gateways.{" "}
            {gatewaysError instanceof Error
              ? gatewaysError.message
              : "Please try again."}
          </Alert>
        )}

        {!gatewaysError && gateways.length === 0 && !isLoadingGateways && (
          <ListingTable.Container>
            {toolbar}
            <ListingTable.EmptyState
              illustration={<ServerCog size={64} />}
              title="No AI gateways available"
              description="Add an AI gateway to deploy this service provider."
              action={
                <Button
                  component={Link}
                  to={generatePath(
                    absoluteRouteMap.children.org.children.gateways.children
                      .add.path,
                    { orgId: orgId ?? "" },
                  )}
                  variant="contained"
                  startIcon={<Rocket size={16} />}
                >
                  Add AI Gateway
                </Button>
              }
            />
          </ListingTable.Container>
        )}

        {!gatewaysError && gateways.length > 0 && (
          <>
            {toolbar}
            {filteredGateways.length === 0 ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<ServerCog size={64} />}
                  title="No gateways match your search"
                  description="Try a different keyword or clear the search filter."
                />
              </ListingTable.Container>
            ) : (
              <Stack pt={4}>
                <ListingTable.Container disablePaper>
                  <ListingTable variant="card">
                    <ListingTable.Head>
                      <ListingTable.Row>
                        <ListingTable.Cell width="300px">
                          Gateway
                        </ListingTable.Cell>
                        <ListingTable.Cell align="center" width="120px">
                          Status
                        </ListingTable.Cell>
                        <ListingTable.Cell align="right" width="140px">
                          Action
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    </ListingTable.Head>
                    <ListingTable.Body>
                      {filteredGateways
                        .slice(
                          page * rowsPerPage,
                          page * rowsPerPage + rowsPerPage,
                        )
                        .map((gateway) => {
                          const displayName =
                            gateway.displayName || gateway.name;
                          const deployed = isDeployedOnGateway(gateway.uuid);
                          const isDeployingThis =
                            deployingGatewayId === gateway.uuid && isDeploying;

                          return (
                            <ListingTable.Row
                              key={gateway.uuid}
                              variant="card"
                            >
                              <ListingTable.Cell>
                                <Stack
                                  direction="row"
                                  alignItems="center"
                                  spacing={2}
                                >
                                  <Avatar
                                    sx={{
                                      bgcolor: "primary.main",
                                      color: "primary.contrastText",
                                      fontSize: 16,
                                      height: 36,
                                      width: 36,
                                      flexShrink: 0,
                                    }}
                                  >
                                    {displayName?.charAt(0)?.toUpperCase() ??
                                      "G"}
                                  </Avatar>
                                  <Box>
                                    <Typography
                                      variant="body2"
                                      fontWeight={500}
                                    >
                                      {displayName}
                                    </Typography>
                                    {gateway.vhost && (
                                      <Typography
                                        variant="caption"
                                        color="text.secondary"
                                        sx={{
                                          fontFamily: "monospace",
                                          display: "block",
                                        }}
                                      >
                                        {gateway.vhost}
                                      </Typography>
                                    )}
                                  </Box>
                                </Stack>
                              </ListingTable.Cell>

                              <ListingTable.Cell align="center">
                                <Chip
                                  label={deployed ? "Deployed" : "Not deployed"}
                                  size="small"
                                  variant="outlined"
                                  color={deployed ? "success" : "default"}
                                />
                              </ListingTable.Cell>

                              <ListingTable.Cell align="right">
                                <Tooltip
                                  title={
                                    deployed
                                      ? "Already deployed"
                                      : isDeployingThis
                                        ? "Deploying..."
                                        : "Deploy to this gateway"
                                  }
                                >
                                  <span>
                                    <Button
                                      variant="contained"
                                      size="small"
                                      startIcon={<Rocket size={16} />}
                                      onClick={() =>
                                        handleDeploy(
                                          gateway.uuid,
                                          displayName,
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
                                  </span>
                                </Tooltip>
                              </ListingTable.Cell>
                            </ListingTable.Row>
                          );
                        })}
                    </ListingTable.Body>
                  </ListingTable>
                </ListingTable.Container>

                {filteredGateways.length > 5 && (
                  <TablePagination
                    component="div"
                    count={filteredGateways.length}
                    page={page}
                    rowsPerPage={rowsPerPage}
                    onPageChange={(_, newPage) => setPage(newPage)}
                    onRowsPerPageChange={(e) => {
                      setRowsPerPage(parseInt(e.target.value, 10));
                      setPage(0);
                    }}
                    rowsPerPageOptions={[5, 10, 25]}
                  />
                )}
              </Stack>
            )}
          </>
        )}
      </Stack>
    </PageLayout>
  );
}

export default DeployLLMProviderPage;

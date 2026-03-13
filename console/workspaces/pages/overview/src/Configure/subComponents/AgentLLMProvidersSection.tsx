/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useMemo, useState } from "react";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Form,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Plus,
  ServerCog,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import { useListLLMProxies } from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { useDeleteLLMProxy } from "@agent-management-platform/api-client";
import type { LLMProxyResponse } from "@agent-management-platform/types";

export function AgentLLMProvidersSection() {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);
  const { addConfirmation } = useConfirmationDialog();

  const {
    data: proxiesData,
    isLoading,
    error,
  } = useListLLMProxies({
    orgName: orgId,
    projName: projectId,
  });

  const { mutate: deleteProxy } = useDeleteLLMProxy();

  const proxies = useMemo(
    () => proxiesData?.proxies ?? [],
    [proxiesData],
  );

  const paginated = useMemo(
    () =>
      proxies.slice(
        page * rowsPerPage,
        page * rowsPerPage + rowsPerPage,
      ),
    [proxies, page, rowsPerPage],
  );

  const addProviderPath =
    orgId && projectId && agentId
      ? generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.llmProviders.children.add.path,
          { orgId, projectId, agentId },
        )
      : "#";

  const handleDelete = (proxy: LLMProxyResponse) => {
    addConfirmation({
      title: "Remove LLM Provider",
      description:
        "Are you sure you want to remove this LLM provider from the project?",
      confirmButtonText: "Remove",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () =>
        deleteProxy({
          orgName: orgId,
          projName: projectId,
          proxyId: proxy.uuid,
        }),
    });
  };

  if (error) {
    return (
      <Form.Section>
        <Form.Header>LLM Providers</Form.Header>
        <Alert
          severity="error"
          icon={<AlertTriangle size={18} />}
          sx={{ mt: 2 }}
        >
          {error instanceof Error
            ? error.message
            : "Failed to load LLM providers. Please try again."}
        </Alert>
      </Form.Section>
    );
  }

  return (
    <Form.Section>
      <Stack
        direction="row"
        alignItems="center"
        justifyContent="space-between"
        sx={{ mb: 2 }}
      >
        <Form.Header>LLM Providers</Form.Header>
        <Button
          component={Link}
          to={addProviderPath}
          variant="contained"
          color="primary"
          size="small"
          startIcon={<Plus size={16} />}
          disabled={!orgId || !projectId || !agentId}
        >
          Add Provider
        </Button>
      </Stack>

      {isLoading ? (
        <Stack spacing={1} sx={{ mt: 2 }}>
          {Array.from({ length: 3 }).map((_, i) => (
            <Stack
              key={i}
              direction="row"
              alignItems="center"
              spacing={2}
              sx={{
                px: 2,
                py: 1.5,
                borderRadius: 1,
                border: "1px solid",
                borderColor: "divider",
                bgcolor: "background.paper",
              }}
            >
              <Skeleton variant="circular" width={36} height={36} />
              <Skeleton variant="text" width={180} height={20} />
              <Skeleton variant="rounded" width={80} height={24} />
            </Stack>
          ))}
        </Stack>
      ) : proxies.length === 0 ? (
        <ListingTable.Container disablePaper sx={{ mt: 2 }}>
          <ListingTable.EmptyState
            illustration={<ServerCog size={64} />}
            title="No LLM providers configured"
            description="Add an LLM provider to use with this agent. Providers are added at the organization level and linked to the project."
          />
        </ListingTable.Container>
      ) : (
        <>
          <ListingTable.Container disablePaper sx={{ mt: 2 }}>
            <ListingTable variant="card">
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell width="300px">
                    Name
                  </ListingTable.Cell>
                  <ListingTable.Cell align="left" width="120px">
                    Version
                  </ListingTable.Cell>
                  <ListingTable.Cell width="140px" align="right">
                    Status
                  </ListingTable.Cell>
                  <ListingTable.Cell width="100px" align="right">
                    Actions
                  </ListingTable.Cell>
                </ListingTable.Row>
              </ListingTable.Head>
              <ListingTable.Body>
                {paginated.map((proxy) => {
                  const displayName =
                    proxy.configuration?.name ??
                    proxy.description ??
                    proxy.uuid;
                  const version =
                    proxy.configuration?.version ?? "—";
                  return (
                    <ListingTable.Row
                      key={proxy.uuid}
                      variant="card"
                      hover
                    >
                      <ListingTable.Cell>
                        <Stack direction="row" alignItems="center" spacing={2}>
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
                            {displayName.charAt(0).toUpperCase()}
                          </Avatar>
                          <Box>
                            <Typography variant="body2" fontWeight={500}>
                              {displayName}
                            </Typography>
                            {proxy.description && (
                              <Typography
                                variant="caption"
                                color="text.secondary"
                                sx={{ display: "block" }}
                              >
                                {proxy.description}
                              </Typography>
                            )}
                          </Box>
                        </Stack>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="left">
                        <Typography variant="body2">{version}</Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="right">
                        <Typography
                          variant="caption"
                          color="text.secondary"
                        >
                          {proxy.status ?? "—"}
                        </Typography>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="right">
                        <Tooltip title="Remove provider">
                          <IconButton
                            color="error"
                            size="small"
                            onClick={() => handleDelete(proxy)}
                          >
                            <Trash size={16} />
                          </IconButton>
                        </Tooltip>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  );
                })}
              </ListingTable.Body>
            </ListingTable>
          </ListingTable.Container>
          {proxies.length > 5 && (
            <TablePagination
              component="div"
              count={proxies.length}
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
        </>
      )}
    </Form.Section>
  );
}

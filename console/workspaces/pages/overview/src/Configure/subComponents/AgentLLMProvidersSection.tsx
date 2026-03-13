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

import { type ReactNode, useMemo, useState } from "react";
import {
  Box,
  Button,
  IconButton,
  ListingTable,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { formatDistanceToNow } from "date-fns";
import {
  AlertTriangle,
  Edit,
  Plus,
  ServerCog,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams } from "react-router-dom";
import {
  useDeleteAgentModelConfig,
  useListAgentModelConfigs,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  type AgentModelConfigListItem,
} from "@agent-management-platform/types";

export function AgentLLMProvidersSection() {
  const { orgId, projectId, agentId } = useParams<{
    orgId: string;
    projectId: string;
    agentId: string;
  }>();
  const [searchValue, setSearchValue] = useState("");
  const { addConfirmation } = useConfirmationDialog();

  const {
    data: configsData,
    isLoading,
    error,
  } = useListAgentModelConfigs(
    {
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    },
    { limit: 100 },
  );

  const { mutate: deleteConfig } = useDeleteAgentModelConfig();

  const configs = useMemo(() => configsData?.configs ?? [], [configsData]);

  const filteredConfigs = useMemo(() => {
    if (!searchValue.trim()) return configs;
    const lower = searchValue.toLowerCase();
    return configs.filter(
      (c) =>
        c.name.toLowerCase().includes(lower) ||
        (c.description ?? "").toLowerCase().includes(lower) ||
        c.type.toLowerCase().includes(lower),
    );
  }, [configs, searchValue]);

  const addProviderPath =
    orgId && projectId && agentId
      ? generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.llmProviders.children.add.path,
          { orgId, projectId, agentId },
        )
      : "#";

  const getEditProviderPath = (configId: string) =>
    orgId && projectId && agentId
      ? generatePath(
          absoluteRouteMap.children.org.children.projects.children.agents
            .children.llmProviders.children.edit.path,
          { orgId, projectId, agentId, configId },
        )
      : "#";

  const handleDelete = (config: AgentModelConfigListItem) => {
    addConfirmation({
      title: "Remove Model Config",
      description:
        "Are you sure you want to remove this LLM provider configuration from the agent?",
      confirmButtonText: "Remove",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () =>
        deleteConfig({
          orgName: orgId,
          projName: projectId,
          agentName: agentId,
          configId: config.uuid,
        }),
    });
  };

  const toolbar = (
    <ListingTable.Toolbar
      showSearch
      searchValue={searchValue}
      onSearchChange={setSearchValue}
      searchPlaceholder="Search by name, description, or type..."
      actions={
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
      }
    />
  );

  const tableHeader = (
    <ListingTable.Head>
      <ListingTable.Row>
        <ListingTable.Cell>Name</ListingTable.Cell>
        <ListingTable.Cell>Description</ListingTable.Cell>
        <ListingTable.Cell>Created</ListingTable.Cell>
        <ListingTable.Cell align="right">Actions</ListingTable.Cell>
      </ListingTable.Row>
    </ListingTable.Head>
  );

  const renderEmptyState = (
    illustration: ReactNode,
    title: string,
    description: string,
  ) => (
    <ListingTable.Row>
      <ListingTable.Cell colSpan={4}>
        <Box sx={{ textAlign: "center", py: 4 }}>
          <Box sx={{ mb: 2 }}>{illustration}</Box>
          <Typography variant="body2" fontWeight={500} gutterBottom>
            {title}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {description}
          </Typography>
        </Box>
      </ListingTable.Cell>
    </ListingTable.Row>
  );

  const getEmptyState = () => {
    if (error) {
      return renderEmptyState(
        <Box component="span" sx={{ color: "error.main" }}>
          <AlertTriangle size={64} />
        </Box>,
        "Failed to load model configs",
        error instanceof Error
          ? error.message
          : "Failed to load model configs. Please try again.",
      );
    }
    if (configs.length === 0) {
      return renderEmptyState(
        <ServerCog size={64} />,
        "No LLM providers configured",
        "Add an LLM provider to use with this agent. Providers are added at the organization level.",
      );
    }
    if (filteredConfigs.length === 0) {
      return renderEmptyState(
        <ServerCog size={64} />,
        "No model configs match your search criteria",
        "Try adjusting your search keywords.",
      );
    }
    return null;
  };

  return (
    <Stack spacing={2}>
      <Typography variant="h6">LLM Providers</Typography>
    <ListingTable.Container>
      {toolbar}
        {isLoading ? (
          <Stack spacing={1} sx={{ mt: 2 }}>
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} variant="rounded" height={56} />
            ))}
          </Stack>
        ) : (
          <ListingTable>
            
            {tableHeader}
            <ListingTable.Body>
              {filteredConfigs.length > 0 ? (
                filteredConfigs.map((config) => (
                  <ListingTable.Row key={config.uuid} hover>
                    <ListingTable.Cell>
                      <Typography variant="body2" fontWeight={500}>
                        {config.name}
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      <Typography variant="body2" color="text.secondary">
                        {config.description ?? "—"}
                      </Typography>
                    </ListingTable.Cell>
                    <ListingTable.Cell>
                      {config.createdAt
                        ? formatDistanceToNow(new Date(config.createdAt), {
                            addSuffix: true,
                          })
                        : "—"}
                    </ListingTable.Cell>
                    <ListingTable.Cell align="right">
                      <Tooltip title="Edit config">
                        <IconButton
                          component={Link}
                          to={getEditProviderPath(config.uuid)}
                          size="small"
                          color="inherit"
                          aria-label={`Edit provider ${config.name || config.uuid}`}
                        >
                          <Edit size={16} />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Remove config">
                        <IconButton
                          color="error"
                          size="small"
                          onClick={() => handleDelete(config)}
                          aria-label={`Remove provider ${config.name || config.uuid}`}
                        >
                          <Trash size={16} />
                        </IconButton>
                      </Tooltip>
                    </ListingTable.Cell>
                  </ListingTable.Row>
                ))
              ) : (
                getEmptyState()
              )}
            </ListingTable.Body>
          </ListingTable>
        )}
      </ListingTable.Container>
      </Stack>
  );
}

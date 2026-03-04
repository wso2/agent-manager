/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
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

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  CircularProgress,
  IconButton,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  AlertTriangle,
  Plus,
  RefreshCcw,
  Search,
  ServerCog,
  Trash,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteLLMProvider,
  useListLLMProviders,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { absoluteRouteMap } from "@agent-management-platform/types";
import { FadeIn } from "@agent-management-platform/views";

export function LLMProviderTable() {
  const navigate = useNavigate();
  const { orgId } = useParams<{ orgId: string }>();
  const [searchValue, setSearchValue] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);
  const [hoveredId, setHoveredId] = useState<string | null>(null);
  const { addConfirmation } = useConfirmationDialog();

  const {
    data: providersList,
    isLoading,
    isRefetching,
    error,
    refetch,
  } = useListLLMProviders({ orgName: orgId });

  const handleRefresh = useCallback(() => {
    refetch();
  }, [refetch]);

  const { mutate: deleteProvider } = useDeleteLLMProvider();

  const { data: templatesData } = useListLLMProviderTemplates({ orgName: orgId });

  const templateLogoMap = useMemo<Record<string, string>>(() => {
    if (!templatesData?.templates) return {};
    return templatesData.templates.reduce<Record<string, string>>((acc, t) => {
      const logoUrl = (t.metadata as { logoUrl?: string } | undefined)?.logoUrl;
      if (logoUrl) acc[t.id] = logoUrl;
      return acc;
    }, {});
  }, [templatesData]);

  const providers = useMemo(
    () => providersList?.providers ?? [],
    [providersList],
  );

  const filteredProviders = useMemo(() => {
    const term = searchValue.trim().toLowerCase();
    if (!term) return providers;
    return providers.filter((p) => {
      const haystack = [
        p.configuration?.name ?? "",
        p.configuration?.version ?? "",
        p.templateHandle,
        p.description ?? "",
        p.status ?? "",
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(term);
    });
  }, [providers, searchValue]);

  useEffect(() => {
    if (page !== 0 && page * rowsPerPage >= filteredProviders.length) {
      setPage(0);
    }
  }, [filteredProviders.length, page, rowsPerPage]);

  const toolbar = (
    <Stack direction="row" spacing={1} alignItems="center">
      <Box flexGrow={1}>
        <SearchBar
          key="search-bar"
          placeholder="Search providers..."
          size="small"
          fullWidth
          value={searchValue}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            setSearchValue(e.target.value)
          }
        />
      </Box>

      <Button
        component={Link}
        to={generatePath(
          absoluteRouteMap.children.org.children.llmProviders.children.add.path,
          { orgId },
        )}
        variant="contained"
        color="primary"
        startIcon={<Plus size={16} />}
      >
        Add Provider
      </Button>
      <Tooltip title="Refresh">
        <IconButton
          size="small"
          onClick={handleRefresh}
          disabled={isRefetching || isLoading}
          aria-label="Refresh providers"
        >
          {isRefetching ? (
            <CircularProgress size={16} />
          ) : (
            <RefreshCcw size={16} />
          )}
        </IconButton>
      </Tooltip>
    </Stack>
  );

  if (error) {
    return (
      <ListingTable.Container>
        {toolbar}
        <Alert
          severity="error"
          icon={<AlertTriangle size={18} />}
          sx={{ alignSelf: "stretch" }}
        >
          {error instanceof Error
            ? error.message
            : "Failed to load providers. Please try again."}
        </Alert>
      </ListingTable.Container>
    );
  }

  if (isLoading) {
    return (
      <ListingTable.Container disablePaper>
        {toolbar}
        <Stack spacing={1} mt={1}>
          {Array.from({ length: 5 }).map((_, i) => (
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
              {/* Name: avatar + text — 300px */}
              <Stack
                direction="row"
                alignItems="center"
                spacing={1.5}
                sx={{ width: 300, flexShrink: 0 }}
              >
                <Skeleton variant="circular" width={36} height={36} />
                <Skeleton variant="text" width={140} height={20} />
              </Stack>

              {/* Version — 100px */}
              <Skeleton
                variant="rounded"
                width={70}
                height={24}
                sx={{ flexShrink: 0 }}
              />

              {/* Description — flexible */}
              <Skeleton variant="text" sx={{ flex: 1 }} height={18} />

              {/* Template — 140px */}
              <Skeleton
                variant="rounded"
                width={100}
                height={24}
                sx={{ flexShrink: 0 }}
              />

              {/* Status — 120px */}
              <Skeleton
                variant="rounded"
                width={72}
                height={24}
                sx={{ flexShrink: 0, ml: "auto" }}
              />
            </Stack>
          ))}
        </Stack>
      </ListingTable.Container>
    );
  }

  if (!providers.length) {
    return (
      <ListingTable.Container>
        {toolbar}
        <ListingTable.EmptyState
          illustration={<ServerCog size={64} />}
          title="No LLM service providers yet"
          description="Add an LLM service provider to start routing AI traffic through the gateway."
        />
      </ListingTable.Container>
    );
  }

  if (!filteredProviders.length) {
    return (
      <Stack spacing={1}>
        {toolbar}
        <ListingTable.Container>
          <ListingTable.EmptyState
            illustration={<Search size={64} />}
            title="No providers match your search"
            description="Try a different keyword or clear the search filter."
          />
        </ListingTable.Container>
      </Stack>
    );
  }

  const paginated = filteredProviders.slice(
    page * rowsPerPage,
    page * rowsPerPage + rowsPerPage,
  );

  return (
    <ListingTable.Container disablePaper>
      {toolbar}
      <ListingTable variant="card">
        <ListingTable.Head>
          <ListingTable.Row>
            <ListingTable.Cell width="300px">Name</ListingTable.Cell>
            <ListingTable.Cell width="100px">Version</ListingTable.Cell>
            <ListingTable.Cell>Description</ListingTable.Cell>
            <ListingTable.Cell width="140px">Template</ListingTable.Cell>
            <ListingTable.Cell align="right" width="120px">
              Status
            </ListingTable.Cell>
          </ListingTable.Row>
        </ListingTable.Head>
        <ListingTable.Body>
          {paginated.map((provider) => {
            const displayName = provider.configuration?.name ?? provider.uuid;
            const version = provider.configuration?.version;
            const statusLabel =
              (provider.status ?? "unknown").charAt(0).toUpperCase() +
              (provider.status ?? "unknown").slice(1);
            const statusColor =
              provider.status === "active"
                ? "success"
                : provider.status === "inactive"
                  ? "default"
                  : "warning";

            return (
              <ListingTable.Row
                key={provider.uuid}
                variant="card"
                hover
                clickable
                onClick={() =>
                  navigate(
                    generatePath(
                      absoluteRouteMap.children.org.children.llmProviders
                        .children.view.path,
                      { orgId, providerId: provider.artifact?.name },
                    ),
                  )
                }
                onMouseEnter={() => setHoveredId(provider.uuid)}
                onMouseLeave={() => setHoveredId(null)}
                onFocus={() => setHoveredId(provider.uuid)}
                onBlur={() => setHoveredId(null)}
              >
                {/* Name */}
                <ListingTable.Cell>
                  <Stack direction="row" alignItems="center" spacing={2}>
                    {templateLogoMap[provider.templateHandle] ? (
                      <Box
                        component="img"
                        src={templateLogoMap[provider.templateHandle]}
                        alt={provider.templateHandle}
                        sx={{
                          width: 36,
                          height: 36,
                          objectFit: "contain",
                          bgcolor: "grey.200",
                          flexShrink: 0,
                          borderRadius: 1,
                        }}
                      />
                    ) : (
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
                    )}
                    <Typography variant="body2" fontWeight={500}>
                      {displayName}
                    </Typography>
                  </Stack>
                </ListingTable.Cell>

                {/* Version */}
                <ListingTable.Cell sx={{ width: "100px", maxWidth: "100px" }}>
                  {version ? (
                    <Chip
                      label={version}
                      size="small"
                      variant="outlined"
                      sx={{ maxWidth: 90 }}
                    />
                  ) : (
                    <Typography variant="caption" color="text.secondary">
                      —
                    </Typography>
                  )}
                </ListingTable.Cell>

                {/* Description */}
                <ListingTable.Cell>
                  <Typography
                    variant="caption"
                    color="text.secondary"
                    noWrap
                    sx={{
                      display: "block",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                    }}
                  >
                    {provider.description ?? ""}
                  </Typography>
                </ListingTable.Cell>

                {/* Template */}
                <ListingTable.Cell sx={{ width: "140px", maxWidth: "140px" }}>
                  <Chip
                    label={provider.templateHandle}
                    size="small"
                    variant="outlined"
                    sx={{ maxWidth: 130 }}
                  />
                </ListingTable.Cell>

                {/* Status + hover actions */}
                <ListingTable.Cell
                  align="right"
                  onClick={(e) => e.stopPropagation()}
                >
                  <Stack
                    direction="row"
                    alignItems="center"
                    spacing={1}
                    justifyContent="flex-end"
                  >
                    {hoveredId === provider.uuid ? (
                      <FadeIn>
                        <Tooltip title="Delete provider">
                          <IconButton
                            color="error"
                            size="small"
                            onClick={() =>
                              addConfirmation({
                                title: "Delete LLM Provider",
                                description:
                                  "Are you sure you want to delete this provider? This action cannot be undone.",
                                confirmButtonText: "Delete",
                                confirmButtonColor: "error",
                                confirmButtonIcon: <Trash size={16} />,
                                onConfirm: () =>
                                  deleteProvider({
                                    orgName: orgId,
                                    providerId: provider.uuid,
                                  }),
                              })
                            }
                          >
                            <Trash size={16} />
                          </IconButton>
                        </Tooltip>
                      </FadeIn>
                    ) : (
                      <Chip
                        label={statusLabel}
                        size="small"
                        variant="outlined"
                        color={statusColor}
                      />
                    )}
                  </Stack>
                </ListingTable.Cell>
              </ListingTable.Row>
            );
          })}
        </ListingTable.Body>
      </ListingTable>

      {filteredProviders.length > 5 && (
        <TablePagination
          component="div"
          count={filteredProviders.length}
          page={page}
          rowsPerPage={rowsPerPage}
          onPageChange={(_e, newPage) => setPage(newPage)}
          onRowsPerPageChange={(e) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
          rowsPerPageOptions={[5, 10, 25]}
        />
      )}
    </ListingTable.Container>
  );
}

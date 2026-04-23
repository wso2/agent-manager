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

import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useListCatalogLLMProviders,
  useListLLMProviderTemplates,
} from "@agent-management-platform/api-client";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import {
  Avatar,
  Box,
  Chip,
  CircularProgress,
  Divider,
  Form,
  ListingTable,
  SearchBar,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Check,
  Circle,
  DoorClosedLocked,
  ExternalLink,
  Plus,
} from "@wso2/oxygen-ui-icons-react";
import type { CatalogLLMProviderEntry } from "@agent-management-platform/types";
import { formatDistanceToNow } from "date-fns";
import { useCallback, useEffect, useMemo, useState } from "react";
import { generatePath, useParams } from "react-router-dom";
import debounce from "lodash/debounce";

interface MonitorLLMProviderDrawerProps {
  open: boolean;
  onClose: () => void;
  selectedProviderName?: string;
  onProviderChange: (name: string | undefined) => void;
}

function getLatestDeployment(
  deployments: CatalogLLMProviderEntry["deployments"],
) {
  if (!deployments?.length) return null;
  return (
    [...deployments].sort(
      (a, b) =>
        new Date(b.deployedAt ?? 0).getTime() -
        new Date(a.deployedAt ?? 0).getTime(),
    )[0] ?? null
  );
}

function getRateLimitingSummary(
  rateLimiting: CatalogLLMProviderEntry["rateLimiting"],
): string {
  if (!rateLimiting) return "Not configured";
  const limits: string[] = [];
  const pl = rateLimiting.providerLevel;
  const cl = rateLimiting.consumerLevel;
  if (pl?.requestLimitCount) limits.push(`${pl.requestLimitCount} req/min`);
  if (pl?.tokenLimitCount) limits.push(`${pl.tokenLimitCount} tokens/min`);
  if (cl?.requestLimitCount)
    limits.push(`Consumer: ${cl.requestLimitCount} req/min`);
  return limits.length > 0 ? limits.join(", ") : "Configured";
}

interface ProviderCardContentProps {
  entry: CatalogLLMProviderEntry;
  isSelected: boolean;
  templateInfo?: { displayName: string; logoUrl?: string } | null;
}

function ProviderCardContent({
  entry,
  isSelected,
  templateInfo,
}: ProviderCardContentProps) {
  const latest = getLatestDeployment(entry.deployments);
  const rateLimitingText = getRateLimitingSummary(entry.rateLimiting);

  return (
    <Stack direction="row" spacing={2} flexGrow={1} alignItems="center">
      <Avatar
        sx={{
          height: 32,
          width: 32,
          backgroundColor: isSelected ? "primary.main" : "secondary.main",
          color: isSelected ? "common.white" : "text.secondary",
          flexShrink: 0,
        }}
      >
        {isSelected ? <Check size={16} /> : <Circle size={16} />}
      </Avatar>
      <Stack spacing={0.25} flexGrow={1}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Typography variant="h6">{entry.name}&nbsp;</Typography>
          {templateInfo && (
            <Tooltip title="Provider template" placement="top" arrow>
              <Chip
                label={templateInfo.displayName}
                size="small"
                variant="outlined"
                icon={
                  templateInfo.logoUrl ? (
                    <Box
                      component="img"
                      src={templateInfo.logoUrl}
                      alt={templateInfo.displayName}
                      sx={{ width: 14, height: 14, borderRadius: "100%" }}
                    />
                  ) : undefined
                }
              />
            </Tooltip>
          )}
        </Stack>
        {latest?.deployedAt && (
          <Typography variant="caption" color="text.secondary">
            Deployed{" "}
            {formatDistanceToNow(new Date(latest.deployedAt), {
              addSuffix: true,
            })}
          </Typography>
        )}
        <Stack direction="column" spacing={0.25} sx={{ mt: 0.5 }}>
          <Typography variant="caption" color="text.secondary">
            Rate Limiting:{" "}
            <Typography
              component="span"
              variant="body2"
              color={entry.rateLimiting ? "text.primary" : "text.disabled"}
            >
              {rateLimitingText}
            </Typography>
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Guardrails:{" "}
            <Typography
              component="span"
              variant="body2"
              color={
                entry.policies?.length ? "text.primary" : "text.disabled"
              }
            >
              {entry.policies?.length ? (
                <Stack
                  component="span"
                  direction="row"
                  spacing={0.25}
                  flexWrap="wrap"
                  alignItems="center"
                  sx={{ display: "inline-flex" }}
                >
                  {entry.policies.slice(0, 3).map((p) => (
                    <Chip key={p} label={p} size="small" variant="outlined" />
                  ))}
                  {entry.policies.length > 3 && (
                    <Tooltip
                      title={entry.policies.join(", ")}
                      placement="top"
                      arrow
                    >
                      <Typography variant="caption" color="text.secondary">
                        {`+${entry.policies.length - 3} more`}
                      </Typography>
                    </Tooltip>
                  )}
                </Stack>
              ) : (
                "None"
              )}
            </Typography>
          </Typography>
        </Stack>
      </Stack>
    </Stack>
  );
}

export function MonitorLLMProviderDrawer({
  open,
  onClose,
  selectedProviderName,
  onProviderChange,
}: MonitorLLMProviderDrawerProps) {
  const { orgId } = useParams<{ orgId: string }>();
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");

  const { data: catalogData, isFetching, refetch } =
    useListCatalogLLMProviders({ orgName: orgId }, { limit: 50 });
  const { data: templatesData } = useListLLMProviderTemplates({
    orgName: orgId,
  });

  const templateMap = useMemo(() => {
    const map = new Map<string, { displayName: string; logoUrl?: string }>();
    for (const t of templatesData?.templates ?? []) {
      map.set(t.name, { displayName: t.name, logoUrl: t.metadata?.logoUrl });
      map.set(t.id, { displayName: t.name, logoUrl: t.metadata?.logoUrl });
    }
    return map;
  }, [templatesData]);

  const debouncedSetSearch = useMemo(
    () => debounce((value: string) => setDebouncedSearch(value), 250),
    [],
  );

  useEffect(() => () => debouncedSetSearch.cancel(), [debouncedSetSearch]);

  useEffect(() => {
    if (open) refetch();
  }, [open, refetch]);

  const allProviders = catalogData?.entries ?? [];

  const filteredProviders = useMemo(() => {
    if (!debouncedSearch.trim()) return allProviders;
    const q = debouncedSearch.toLowerCase();
    return allProviders.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        (p.template ?? "").toLowerCase().includes(q) ||
        (templateMap.get(p.template ?? "")?.displayName ?? "")
          .toLowerCase()
          .includes(q),
    );
  }, [allProviders, debouncedSearch, templateMap]);

  const addProviderPath = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.llmProviders.children.add.path,
        { orgId },
      )
    : null;

  const handleSearchChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      setSearch(event.target.value);
      debouncedSetSearch(event.target.value);
    },
    [debouncedSetSearch],
  );

  const handleSelect = useCallback(
    (providerHandle: string) => {
      onProviderChange(providerHandle);
      onClose();
    },
    [onProviderChange, onClose],
  );

  return (
    <DrawerWrapper open={open} onClose={onClose} maxWidth={520}>
      <DrawerHeader
        icon={<DoorClosedLocked size={24} />}
        title="Select LLM Provider"
        onClose={onClose}
      />
      <DrawerContent>
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            Select the LLM provider for all LLM-judge evaluators in this
            monitor.
          </Typography>
          <SearchBar
            placeholder="Search providers"
            size="small"
            fullWidth
            value={search}
            onChange={handleSearchChange}
          />
          {isFetching && (
            <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
              <CircularProgress size={32} />
            </Box>
          )}
          {!isFetching && filteredProviders.length === 0 && (
            <ListingTable.EmptyState
              title={
                search.trim()
                  ? "No providers match your search"
                  : "No LLM providers configured"
              }
              description={
                search.trim()
                  ? "Try a different keyword."
                  : "Add an LLM service provider to get started."
              }
            />
          )}
          {!isFetching && filteredProviders.length > 0 && (
            <Stack spacing={1}>
              {filteredProviders.map((entry) => {
                const isSelected = entry.handle === selectedProviderName;
                const templateInfo = templateMap.get(entry.template ?? "");
                return (
                  <Form.CardButton
                    key={entry.uuid}
                    onClick={() => handleSelect(entry.handle)}
                    selected={isSelected}
                  >
                    <Form.CardContent>
                      <ProviderCardContent
                        entry={entry}
                        isSelected={isSelected}
                        templateInfo={templateInfo}
                      />
                    </Form.CardContent>
                  </Form.CardButton>
                );
              })}
            </Stack>
          )}
          {addProviderPath && (
            <>
              <Divider />
              <Box
                component="a"
                href={addProviderPath}
                target="_blank"
                rel="noopener noreferrer"
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  color: "primary.main",
                  textDecoration: "none",
                  cursor: "pointer",
                  "&:hover": { textDecoration: "underline" },
                }}
              >
                <Plus size={16} />
                <Typography variant="body2" color="primary">
                  Add LLM Provider
                </Typography>
                <ExternalLink size={14} />
              </Box>
            </>
          )}
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

export default MonitorLLMProviderDrawer;

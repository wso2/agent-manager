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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useRef, useState } from "react";
import { generatePath, useParams } from "react-router-dom";
import { absoluteRouteMap, withBasePath } from "@agent-management-platform/types";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
} from "@agent-management-platform/views";
import { Box, Divider, Form, ListingTable, SearchBar, Stack, Typography } from "@wso2/oxygen-ui";
import { ExternalLink, Link, Plus, Search } from "@wso2/oxygen-ui-icons-react";
import { ProviderDisplay } from "./AddLLMProvider.Component";

export type ProviderOption = {
  uuid: string;
  id: string;
  name: string;
  template?: string;
  version?: string;
  deployments?: unknown[];
  security?: unknown;
  rateLimiting?: unknown;
  policies?: unknown[];
};

interface ProviderSelectDrawerProps {
  open: boolean;
  onClose: () => void;
  providers: ProviderOption[];
  templateMap: Map<string, { displayName: string; logoUrl?: string }>;
  selectedUuid: string | undefined;
  subtitle?: string;
  onSelect: (uuid: string) => void;
}

export const ProviderSelectDrawer: React.FC<ProviderSelectDrawerProps> = ({
  open,
  onClose,
  providers,
  templateMap,
  selectedUuid,
  subtitle,
  onSelect,
}) => {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchQuery, setSearchQuery] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Rendered as a raw <a target="_blank">, which bypasses the router basename.
  const addProviderPath = orgId
    ? withBasePath(generatePath(
        absoluteRouteMap.children.org.children.llmProviders.children.add.path,
        { orgId },
      ))
    : null;

  const filtered = providers.filter((p) => {
    if (!debouncedSearch.trim()) return true;
    const q = debouncedSearch.toLowerCase();
    return (
      p.name.toLowerCase().includes(q) ||
      (p.template ?? "").toLowerCase().includes(q) ||
      (templateMap.get(p.template ?? "")?.displayName ?? "").toLowerCase().includes(q)
    );
  });

  return (
    <DrawerWrapper open={open} onClose={onClose} minWidth={740} maxWidth={740}>
      <DrawerHeader
        icon={<Link size={24} />}
        title="Select LLM Service"
        onClose={onClose}
      />
      <DrawerContent>
        <Stack>
          {subtitle && (
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              {subtitle}
            </Typography>
          )}
          <SearchBar
            placeholder="Search providers"
            size="small"
            fullWidth
            value={searchQuery}
            onChange={(e) => {
              const val = e.target.value;
              setSearchQuery(val);
              if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
              searchTimerRef.current = setTimeout(() => setDebouncedSearch(val), 250);
            }}
            sx={{ mb: 1 }}
          />
          <Stack spacing={1} sx={{ flex: 1, overflowY: "auto" }}>
            {filtered.length === 0 ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<Search size={64} />}
                  title={
                    debouncedSearch.trim()
                      ? "No service providers match your search"
                      : "No service providers available"
                  }
                  description={
                    debouncedSearch.trim()
                      ? "Try a different keyword or clear the search filter."
                      : "No service providers are available in the catalog."
                  }
                />
              </ListingTable.Container>
            ) : (
              filtered.map((p) => {
                const isSelected = selectedUuid === p.uuid;
                return (
                  <Form.CardButton
                    key={p.uuid}
                    onClick={() => {
                      onSelect(p.uuid);
                      onClose();
                    }}
                    selected={isSelected}
                    aria-label={`${p.name}. ${isSelected ? "Selected" : "Click to select"}`}
                  >
                    <Form.CardContent>
                      <ProviderDisplay
                        provider={p as Parameters<typeof ProviderDisplay>[0]["provider"]}
                        isSelected={isSelected}
                        templateInfo={templateMap.get(p.template ?? "")}
                      />
                    </Form.CardContent>
                  </Form.CardButton>
                );
              })
            )}
          </Stack>
          {addProviderPath && (
            <>
              <Divider sx={{ mt: 2 }} />
              <Box
                component="a"
                href={addProviderPath}
                target="_blank"
                rel="noopener noreferrer"
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  pt: 1.5,
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
};

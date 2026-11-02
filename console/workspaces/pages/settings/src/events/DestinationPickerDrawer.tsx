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

import { useState } from "react";
import {
  Avatar,
  Box,
  CircularProgress,
  Divider,
  Form,
  ListingTable,
  SearchBar,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { Check, Circle, Plus, Search, Send } from "@wso2/oxygen-ui-icons-react";
import { DrawerContent, DrawerHeader, DrawerWrapper } from "@agent-management-platform/views";
import type {
  DestinationKindService,
  EventDestinationSummary,
} from "@agent-management-platform/types";
import { DestinationIcon, HealthChip, lookOf } from "./eventsUi";

/** A destination as the picker lists it. */
export interface PickableDestination extends EventDestinationSummary {
  /** Who may use it, when the caller may see that (organization level). */
  availableTo?: string;
  description?: string;
}

export interface DestinationPickerDrawerProps {
  open: boolean;
  onClose: () => void;
  destinations: PickableDestination[];
  services: DestinationKindService[];
  isLoading: boolean;
  selectedId?: string;
  onSelect: (id: string) => void;
  /** Set for callers who may add destinations: opens the add form. */
  onAdd?: () => void;
}

/** Pick the destination a subscription sends to, like picking an LLM provider. */
export function DestinationPickerDrawer({
  open,
  onClose,
  destinations,
  services,
  isLoading,
  selectedId,
  onSelect,
  onAdd,
}: DestinationPickerDrawerProps) {
  const [search, setSearch] = useState("");
  const q = search.trim().toLowerCase();
  const filtered = destinations.filter(
    (d) =>
      !q ||
      d.name.toLowerCase().includes(q) ||
      lookOf(d.kind, d.service, services).label.toLowerCase().includes(q) ||
      (d.description ?? "").toLowerCase().includes(q),
  );

  return (
    <DrawerWrapper open={open} onClose={onClose} minWidth={620} maxWidth={620}>
      <DrawerHeader icon={<Send size={22} />} title="Select destination" onClose={onClose} />
      <DrawerContent>
        <Stack sx={{ height: "100%" }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
            Events go to one destination. Only destinations available to this level are listed.
          </Typography>
          <SearchBar
            placeholder="Search destinations"
            size="small"
            fullWidth
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            sx={{ mb: 1.5 }}
          />
          <Stack spacing={1} sx={{ flex: 1, overflowY: "auto" }}>
            {isLoading ? (
              <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
                <CircularProgress size={28} />
              </Box>
            ) : filtered.length === 0 ? (
              <ListingTable.Container>
                <ListingTable.EmptyState
                  illustration={<Search size={64} />}
                  title={q ? "No destinations match your search" : "No destinations available"}
                  description={
                    q
                      ? "Try a different keyword or clear the search."
                      : onAdd
                        ? "Add one below, then send this subscription's events to it."
                        : "Ask an organization admin to add a destination, or to make one available to this project."
                  }
                />
              </ListingTable.Container>
            ) : (
              filtered.map((d) => {
                const selected = d.id === selectedId;
                const look = lookOf(d.kind, d.service, services);
                return (
                  <Form.CardButton
                    key={d.id}
                    selected={selected}
                    aria-label={`${d.name}. ${selected ? "Selected" : "Click to select"}`}
                    onClick={() => {
                      onSelect(d.id);
                      onClose();
                    }}
                    sx={{ width: "100%", justifyContent: "flex-start" }}
                  >
                    <Form.CardContent sx={{ width: "100%" }}>
                      <Stack direction="row" spacing={1.5} alignItems="center" sx={{ minWidth: 0 }}>
                        <Avatar
                          sx={{
                            width: 28,
                            height: 28,
                            bgcolor: selected ? "primary.main" : "action.selected",
                            color: selected ? "primary.contrastText" : "text.secondary",
                          }}
                        >
                          {selected ? <Check size={14} /> : <Circle size={14} />}
                        </Avatar>
                        <DestinationIcon kind={d.kind} service={d.service} size={30} />
                        <Box sx={{ minWidth: 0, flex: 1 }}>
                          <Stack
                            direction="row"
                            spacing={1}
                            alignItems="center"
                            sx={{ minWidth: 0 }}
                          >
                            <Typography variant="subtitle2" noWrap title={d.name}>
                              {d.name}
                            </Typography>
                            <Typography variant="caption" color="text.secondary" noWrap>
                              {look.label}
                            </Typography>
                          </Stack>
                          {(d.availableTo || d.description) && (
                            <Typography
                              variant="caption"
                              color="text.secondary"
                              noWrap
                              display="block"
                            >
                              {[d.availableTo && `Available to ${d.availableTo}`, d.description]
                                .filter(Boolean)
                                .join(" · ")}
                            </Typography>
                          )}
                        </Box>
                        <HealthChip health={d.health} />
                      </Stack>
                    </Form.CardContent>
                  </Form.CardButton>
                );
              })
            )}
          </Stack>
          <Divider sx={{ mt: 2 }} />
          {onAdd ? (
            <Box
              component="button"
              type="button"
              onClick={onAdd}
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 1,
                pt: 1.5,
                color: "primary.main",
                background: "none",
                border: 0,
                cursor: "pointer",
                font: "inherit",
                px: 0,
                "&:hover": { textDecoration: "underline" },
              }}
            >
              <Plus size={16} />
              <Typography variant="body2" color="primary">
                Add destination
              </Typography>
            </Box>
          ) : (
            <Typography variant="caption" color="text.secondary" sx={{ pt: 1.5 }}>
              Only organization admins can add destinations.
            </Typography>
          )}
        </Stack>
      </DrawerContent>
    </DrawerWrapper>
  );
}

export default DestinationPickerDrawer;

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

// Module marketplace (/org/:orgId/ui-modules, from the user menu): every
// console module as a card with an Install / Uninstall button and a link to
// its details page.
//
// MOCK: installing only changes in-memory state; see moduleUi.tsx.
//
// This page belongs to the shell, not to a module, so it can never be
// uninstalled.

import { useMemo, useState, type ChangeEvent } from "react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Grid,
  SearchBar,
  Stack,
  Tab,
  Tabs,
  Typography,
} from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { useUIModuleManager, type UIModuleStatus } from "../../plugins";
import {
  InstallButton,
  listingFor,
  marketplaceState,
  StatusChip,
} from "./moduleUi";
import { MODULE_SETUP_ROUTE } from "./routes";

type Filter = "all" | "installed" | "available";

const isInstalled = (module: UIModuleStatus) => {
  const state = marketplaceState(module);
  return state === "installed" || state === "built-in";
};

/** Can be installed: not installed, and not turned off in ui-modules.js. */
const isAvailable = (module: UIModuleStatus) =>
  !isInstalled(module) && marketplaceState(module) !== "turned-off";

function ModuleCard({ module, onDetails }: { module: UIModuleStatus; onDetails: () => void }) {
  const listing = listingFor(module.id);
  const Icon = listing.icon;
  const state = marketplaceState(module);
  return (
    <Card variant="outlined" sx={{ height: "100%", display: "flex", flexDirection: "column" }}>
      <CardContent sx={{ flex: 1, display: "flex", flexDirection: "column", gap: 1.5 }}>
        <Stack direction="row" spacing={1.5} alignItems="center">
          <Avatar variant="rounded" sx={{ bgcolor: "primary.main", width: 44, height: 44 }}>
            <Icon size={24} />
          </Avatar>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography variant="h6" noWrap>
              {module.name}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              by {listing.publisher} · v{listing.version}
            </Typography>
          </Box>
          <StatusChip state={state} />
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>
          {module.description}
        </Typography>
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
          <Chip size="small" label={listing.category} />
        </Stack>
        <Stack direction="row" spacing={1} sx={{ mt: "auto", pt: 1 }}>
          <InstallButton module={module} size="small" />
          <Button size="small" variant="text" onClick={onDetails}>
            Details
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function ModuleManager() {
  const { orgId } = useParams();
  const navigate = useNavigate();
  const { modules, hasChanges, resetChanges } = useUIModuleManager();
  const [filter, setFilter] = useState<Filter>("all");
  const [search, setSearch] = useState("");

  const visible = useMemo(() => {
    const query = search.trim().toLowerCase();
    return modules.filter((module) => {
      if (filter === "installed" && !isInstalled(module)) return false;
      if (filter === "available" && !isAvailable(module)) return false;
      if (!query) return true;
      return [module.name, module.description, listingFor(module.id).category]
        .join(" ")
        .toLowerCase()
        .includes(query);
    });
  }, [modules, filter, search]);

  const installedCount = modules.filter(isInstalled).length;
  const availableCount = modules.filter(isAvailable).length;

  return (
    <PageLayout
      title="Marketplace"
      description="Add capabilities to the console. Install a module to add its pages; uninstall it to remove them."
      disableIcon
      actions={
        <Button variant="outlined" disabled={!hasChanges} onClick={resetChanges}>
          Reset
        </Button>
      }
    >
      <Stack spacing={2}>
        <Alert severity="info">
          Preview: installing and uninstalling here only changes this browser tab and is undone
          when you reload. Nothing is deployed to the cluster.
        </Alert>
        <Stack
          direction={{ xs: "column", md: "row" }}
          spacing={2}
          alignItems={{ md: "center" }}
          justifyContent="space-between"
        >
          <Box sx={{ width: { xs: "100%", md: 400 } }}>
            <SearchBar
              placeholder="Search modules"
              size="small"
              fullWidth
              value={search}
              onChange={(event: ChangeEvent<HTMLInputElement>) => setSearch(event.target.value)}
            />
          </Box>
          <Tabs value={filter} onChange={(_, value: Filter) => setFilter(value)}>
            <Tab value="all" label={`All (${modules.length})`} />
            <Tab value="installed" label={`Installed (${installedCount})`} />
            <Tab value="available" label={`Available (${availableCount})`} />
          </Tabs>
        </Stack>
        {visible.length === 0 ? (
          <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: "center" }}>
            No modules match.
          </Typography>
        ) : (
          <Grid container spacing={2}>
            {visible.map((module) => (
              <Grid key={module.id} size={{ xs: 12, md: 6, xl: 4 }}>
                <ModuleCard
                  module={module}
                  onDetails={() =>
                    navigate(
                      generatePath(MODULE_SETUP_ROUTE, { orgId: orgId ?? "", moduleId: module.id }),
                    )
                  }
                />
              </Grid>
            ))}
          </Grid>
        )}
      </Stack>
    </PageLayout>
  );
}

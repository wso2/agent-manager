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

// A module's details page in the AI module manager (/org/:orgId/ui-modules/:moduleId):
// what the module adds, its backend setup steps, and the same Install /
// Uninstall action as its card in the AI module manager, plus the module's own setup
// content if it has any.
//
// MOCK: installing only changes in-memory state; see moduleUi.tsx.

import { Suspense } from "react";
import { generatePath, useParams } from "react-router-dom";
import {
  Avatar,
  Box,
  Card,
  CardContent,
  Chip,
  Divider,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import {
  CodeBlock,
  ErrorPages,
  getAmpChartRegistry,
  getInstalledChartVersion,
} from "@agent-management-platform/shared-component";
import { useUIModuleManager, type UIModuleStatus } from "../../plugins";
import { LoadingFallback } from "../../components/LoadingFallback";
import {
  countLabel,
  InstallButton,
  listingFor,
  marketplaceState,
  StatusChip,
} from "./moduleUi";
import { MODULE_MANAGER_ROUTE } from "./routes";

/** Public chart registry the install scripts default to. */
const PUBLIC_CHART_REGISTRY = "ghcr.io/wso2";

function fillCommand(command: string) {
  return command
    .replaceAll("{chartRegistry}", getAmpChartRegistry() || PUBLIC_CHART_REGISTRY)
    .replaceAll("{chartVersion}", getInstalledChartVersion() || "<chart-version>");
}

function IncludedCard({ module }: { module: UIModuleStatus }) {
  return (
    <Card variant="outlined">
      <CardContent>
        <Typography variant="h6">What&apos;s included</Typography>
        <Typography variant="body2" color="text.secondary">
          Installing {module.name} adds these to the console.
        </Typography>
        <Divider sx={{ my: 1.5 }} />
        <Stack divider={<Divider flexItem />} spacing={1}>
          {module.plugins.map((plugin) => {
            const contents = [
              plugin.pages ? countLabel(plugin.pages, "page") : null,
              plugin.navItems ? countLabel(plugin.navItems, "menu item") : null,
              plugin.panels ? countLabel(plugin.panels, "panel") : null,
            ].filter(Boolean);
            return (
              <Box key={plugin.id}>
                <Typography variant="body2" color="text.primary">
                  {plugin.name}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {contents.join(", ")}
                </Typography>
              </Box>
            );
          })}
        </Stack>
      </CardContent>
    </Card>
  );
}

function BackendCard({ module }: { module: UIModuleStatus }) {
  const installation = module.installation;
  if (!installation?.steps.length) return null;
  return (
    <Card variant="outlined">
      <CardContent>
        <Typography variant="h6">Backend setup</Typography>
        <Typography variant="body2" color="text.secondary">
          {module.name} needs a backend in the cluster
          {installation.chart ? (
            <>
              {" "}
              (Helm chart <code>{installation.chart}</code>)
            </>
          ) : null}
          . Run these steps for its pages to have data.
        </Typography>
        <Divider sx={{ my: 1.5 }} />
        <Stack spacing={2}>
          {installation.steps.map((step, index) => (
            <Box key={step.title}>
              <Typography variant="subtitle2">
                {index + 1}. {step.title}
              </Typography>
              {step.description && (
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  {step.description}
                </Typography>
              )}
              {step.command && <CodeBlock code={fillCommand(step.command)} language="bash" />}
            </Box>
          ))}
        </Stack>
      </CardContent>
    </Card>
  );
}

export function ModuleSetup() {
  const { orgId, moduleId } = useParams();
  const { modules } = useUIModuleManager();
  const module = modules.find((m) => m.id === moduleId);
  if (!module) {
    return <ErrorPages.NotFound />;
  }
  const listing = listingFor(module.id);
  const Icon = listing.icon;
  const { SetupContent } = module;
  return (
    <PageLayout
      title={module.name}
      description={module.description}
      disableIcon
      backHref={generatePath(MODULE_MANAGER_ROUTE, { orgId: orgId ?? "" })}
      backLabel="Back to AI module manager"
      actions={<InstallButton module={module} />}
    >
      <Stack spacing={2}>
        <Stack direction="row" spacing={1.5} alignItems="center">
          <Avatar variant="rounded" sx={{ bgcolor: "primary.main", width: 40, height: 40 }}>
            <Icon size={22} />
          </Avatar>
          <Typography variant="body2" color="text.secondary">
            by {listing.publisher} · v{listing.version}
          </Typography>
          <Chip size="small" label={listing.category} />
          <StatusChip state={marketplaceState(module)} />
        </Stack>
        <IncludedCard module={module} />
        <BackendCard module={module} />
        {SetupContent && (
          <Suspense fallback={<LoadingFallback />}>
            <SetupContent />
          </Suspense>
        )}
      </Stack>
    </PageLayout>
  );
}

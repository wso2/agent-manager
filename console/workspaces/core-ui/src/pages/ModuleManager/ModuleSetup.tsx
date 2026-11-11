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

// A module's setup page (/org/:orgId/ui-modules/:moduleId). Each module gets
// one: it walks through installing the module's backend, turns the module and
// its pages on and off, and shows the module's own setup content if it has
// any. Install and on/off changes are kept in memory (PluginProvider state).

import { Suspense, useState } from "react";
import { generatePath, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Divider,
  FormControlLabel,
  Stack,
  Switch,
  Typography,
} from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import {
  CodeBlock,
  ErrorPages,
  getAmpChartRegistry,
  getInstalledChartVersion,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";
import type { UIModuleInstallStep } from "@agent-management-platform/types";
import {
  useUIModuleManager,
  type UIModulePluginStatus,
  type UIModuleStatus,
} from "../../plugins";
import { LoadingFallback } from "../../components/LoadingFallback";
import {
  ActivationChip,
  ConfigChip,
  countLabel,
  InstallChip,
  pagesSummary,
} from "./moduleUi";
import { MODULE_MANAGER_ROUTE } from "./routes";

type Manager = ReturnType<typeof useUIModuleManager>;

/** Public chart registry the install scripts default to. */
const PUBLIC_CHART_REGISTRY = "ghcr.io/wso2";

function fillCommand(command: string) {
  return command
    .replaceAll("{chartRegistry}", getAmpChartRegistry() || PUBLIC_CHART_REGISTRY)
    .replaceAll("{chartVersion}", getInstalledChartVersion() || "<chart-version>");
}

// ---------------------------------------------------------------------------
// Installation

function InstallSteps({
  steps,
  done,
  onToggle,
}: {
  steps: UIModuleInstallStep[];
  done?: boolean[];
  onToggle?: (index: number, checked: boolean) => void;
}) {
  return (
    <Stack spacing={2}>
      {steps.map((step, index) => (
        <Box key={step.title}>
          <Stack direction="row" alignItems="center" spacing={1}>
            {onToggle ? (
              <FormControlLabel
                control={
                  <Checkbox
                    size="small"
                    checked={!!done?.[index]}
                    onChange={(_, checked) => onToggle(index, checked)}
                  />
                }
                label={
                  <Typography variant="subtitle2">
                    {index + 1}. {step.title}
                  </Typography>
                }
              />
            ) : (
              <Typography variant="subtitle2">
                {index + 1}. {step.title}
              </Typography>
            )}
          </Stack>
          {step.description && (
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              {step.description}
            </Typography>
          )}
          {step.command && <CodeBlock code={fillCommand(step.command)} language="bash" />}
        </Box>
      ))}
    </Stack>
  );
}

function InstallationCard({ module, manager }: { module: UIModuleStatus; manager: Manager }) {
  const { addConfirmation } = useConfirmationDialog();
  const steps = module.installation?.steps ?? [];
  const [done, setDone] = useState<boolean[]>(() => steps.map(() => false));
  const allDone = done.every(Boolean);

  const uninstall = () =>
    addConfirmation({
      analytics: { entity: "ui-module", action: "uninstall" },
      title: `Uninstall ${module.name}?`,
      description:
        "Its pages are hidden for the rest of this session. This does not remove " +
        (module.installation?.chart
          ? `the ${module.installation.chart} Helm release; uninstall that separately.`
          : "anything from the cluster."),
      confirmButtonText: "Uninstall",
      confirmButtonColor: "error",
      onConfirm: () => manager.setInstallPhase(module.id, "not-installed"),
    });

  const detectionNote = (() => {
    if (module.core) return "Part of the console core. Always installed.";
    if (!module.installation) return "Runs entirely in the console; there is nothing to install.";
    if (module.installChanged) return "Set on this page.";
    if (module.detected === null) return "No automatic check; counted as installed until marked otherwise.";
    if (module.detected === "checking") return "Checking with the agent manager service.";
    return "Detected automatically.";
  })();

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" alignItems="center" spacing={1}>
          <Typography variant="h6">Installation</Typography>
          <InstallChip module={module} />
        </Stack>
        <Typography variant="body2" color="text.secondary">
          {detectionNote}
          {module.installation?.chart && (
            <>
              {" "}
              Helm chart: <code>{module.installation.chart}</code>.
            </>
          )}
        </Typography>

        {!module.core && (
          <>
            <Divider sx={{ my: 2 }} />

            {module.installPhase === "checking" && <LoadingFallback />}

            {module.installPhase === "not-installed" && (
              <Stack spacing={2} alignItems="flex-start">
                <Typography variant="body2">
                  {module.name} is not installed, so its pages are hidden.
                  {steps.length ? " Start the installation to see the steps." : ""}
                </Typography>
                <Stack direction="row" spacing={1}>
                  <Button
                    variant="contained"
                    onClick={() =>
                      manager.setInstallPhase(module.id, steps.length ? "installing" : "installed")
                    }
                  >
                    {steps.length ? "Start installation" : "Install"}
                  </Button>
                  {module.installChanged && (
                    <Button variant="text" onClick={() => manager.setInstallPhase(module.id, undefined)}>
                      Use detected state
                    </Button>
                  )}
                </Stack>
              </Stack>
            )}

            {module.installPhase === "installing" && (
              <Stack spacing={2}>
                <Typography variant="body2">
                  Run each step, tick it off, then finish the installation. The module&apos;s
                  pages appear once it is installed.
                </Typography>
                <InstallSteps
                  steps={steps}
                  done={done}
                  onToggle={(index, checked) =>
                    setDone((prev) => prev.map((value, i) => (i === index ? checked : value)))
                  }
                />
                <Stack direction="row" spacing={1}>
                  <Button
                    variant="contained"
                    disabled={!allDone}
                    onClick={() => manager.setInstallPhase(module.id, "installed")}
                  >
                    Finish installation
                  </Button>
                  <Button variant="outlined" onClick={() => manager.setInstallPhase(module.id, undefined)}>
                    Cancel
                  </Button>
                </Stack>
              </Stack>
            )}

            {module.installPhase === "installed" && (
              <Stack spacing={2} alignItems="flex-start">
                {module.installChanged && module.detected === "not-installed" && (
                  <Alert severity="warning">
                    Marked as installed here, but the agent manager service does not report it
                    yet. Its pages may not work until the steps below are done.
                  </Alert>
                )}
                {steps.length > 0 && (
                  <Box sx={{ width: "100%" }}>
                    <Typography variant="subtitle2" sx={{ mb: 1 }}>
                      Install steps
                    </Typography>
                    <InstallSteps steps={steps} />
                  </Box>
                )}
                <Stack direction="row" spacing={1}>
                  <Button variant="outlined" color="error" onClick={uninstall}>
                    Uninstall
                  </Button>
                  {module.installChanged && (
                    <Button variant="text" onClick={() => manager.setInstallPhase(module.id, undefined)}>
                      Use detected state
                    </Button>
                  )}
                </Stack>
              </Stack>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Pages

function PluginRow({
  module,
  plugin,
  disabled,
  onToggle,
}: {
  module: UIModuleStatus;
  plugin: UIModulePluginStatus;
  disabled: boolean;
  onToggle: (enabled: boolean) => void;
}) {
  const contents = [
    plugin.pages ? countLabel(plugin.pages, "page") : null,
    plugin.navItems ? countLabel(plugin.navItems, "menu item") : null,
    plugin.panels ? countLabel(plugin.panels, "panel") : null,
  ].filter(Boolean);
  return (
    <Stack direction="row" alignItems="center" spacing={2} sx={{ py: 1 }}>
      <Switch
        size="small"
        checked={!disabled && plugin.enabled}
        disabled={disabled}
        onChange={(_, checked) => onToggle(checked)}
        // MUI 7 ignores the deprecated inputProps; slotProps reaches the input
        // (and replaces its defaults, so the switch role is set again).
        slotProps={{
          input: {
            role: "switch",
            "aria-label": `${plugin.enabled ? "Turn off" : "Turn on"} ${plugin.name}`,
          },
        }}
      />
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="body2" color="text.primary">
          {plugin.name}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {module.id}/{plugin.id}
          {contents.length ? ` · ${contents.join(", ")}` : ""}
        </Typography>
      </Box>
      <Stack direction="row" spacing={1}>
        <ConfigChip enabled={plugin.enabled} enabledByConfig={plugin.enabledByConfig} />
        {!disabled && plugin.enabled && <ActivationChip activation={plugin.activation} />}
      </Stack>
    </Stack>
  );
}

function PagesCard({ module, manager }: { module: UIModuleStatus; manager: Manager }) {
  const installed = module.installPhase === "installed" || module.installPhase === "checking";
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" alignItems="flex-start" spacing={2}>
          {!module.core && (
            <Switch
              checked={module.enabled}
              disabled={!installed}
              onChange={(_, checked) => manager.setModuleEnabled(module.id, checked)}
              slotProps={{
                input: {
                  role: "switch",
                  "aria-label": `${module.enabled ? "Turn off" : "Turn on"} ${module.name}`,
                },
              }}
            />
          )}
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <Typography variant="h6">Pages</Typography>
              {!module.core && (
                <ConfigChip enabled={module.enabled} enabledByConfig={module.enabledByConfig} />
              )}
            </Stack>
            <Typography variant="body2" color="text.secondary">
              {module.core
                ? "Core module: it is always on, but single pages can be turned off."
                : pagesSummary(module)}
            </Typography>
          </Box>
        </Stack>
        <Divider sx={{ my: 1.5 }} />
        <Box sx={{ pl: 1 }}>
          {module.plugins.map((plugin) => (
            <PluginRow
              key={plugin.id}
              module={module}
              plugin={plugin}
              disabled={!installed || !module.enabled}
              onToggle={(enabled) => manager.setPluginEnabled(module.id, plugin.id, enabled)}
            />
          ))}
        </Box>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------

export function ModuleSetup() {
  const { orgId, moduleId } = useParams();
  const manager = useUIModuleManager();
  const module = manager.modules.find((m) => m.id === moduleId);
  if (!module) {
    return <ErrorPages.NotFound />;
  }
  const { SetupContent } = module;
  return (
    <PageLayout
      title={module.name}
      description={module.description}
      disableIcon
      backHref={generatePath(MODULE_MANAGER_ROUTE, { orgId: orgId ?? "" })}
      backLabel="Back to Module manager"
    >
      <Stack spacing={2}>
        <InstallationCard key={module.id} module={module} manager={manager} />
        <PagesCard module={module} manager={manager} />
        {SetupContent && (
          <Suspense fallback={<LoadingFallback />}>
            <SetupContent />
          </Suspense>
        )}
      </Stack>
    </PageLayout>
  );
}

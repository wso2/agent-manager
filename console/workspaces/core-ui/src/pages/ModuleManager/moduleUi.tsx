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

// Status chips shared by the Module manager overview and module setup pages.

import { Chip, Tooltip } from "@wso2/oxygen-ui";
import type { UIModuleInstallPhase } from "@agent-management-platform/types";
import type { UIModulePluginStatus, UIModuleStatus } from "../../plugins";

export function countLabel(count: number, singular: string) {
  return `${count} ${singular}${count === 1 ? "" : "s"}`;
}

const INSTALL_LABEL: Record<UIModuleInstallPhase, string> = {
  installed: "Installed",
  "not-installed": "Not installed",
  installing: "Installing",
  checking: "Checking",
};

export function InstallChip({ module }: { module: UIModuleStatus }) {
  if (module.core) {
    return <Chip size="small" variant="outlined" label="Core" />;
  }
  const phase = module.installPhase;
  const color =
    phase === "installed" ? "success" : phase === "not-installed" ? "default" : "info";
  return (
    <Chip
      size="small"
      color={color}
      variant={phase === "installed" ? "outlined" : "filled"}
      label={INSTALL_LABEL[phase]}
    />
  );
}

/** Shows when the current state differs from, or comes from, the runtime config. */
export function ConfigChip({
  enabled,
  enabledByConfig,
}: {
  enabled: boolean;
  enabledByConfig: boolean;
}) {
  if (enabled !== enabledByConfig) {
    return (
      <Tooltip title="Changed here. The change lasts until you reload the page.">
        <Chip size="small" color="info" label="Changed" />
      </Tooltip>
    );
  }
  if (!enabledByConfig) {
    return (
      <Tooltip title="Turned off in the runtime config (uiModules.disabled).">
        <Chip size="small" variant="outlined" label="Off in config" />
      </Tooltip>
    );
  }
  return null;
}

/** Explains why an enabled page might still not show for this user. */
export function ActivationChip({
  activation,
}: {
  activation: UIModulePluginStatus["activation"];
}) {
  if (activation === null || activation === "always") return null;
  if (activation === undefined) {
    return <Chip size="small" variant="outlined" label="Checking" />;
  }
  return activation ? (
    <Chip size="small" color="success" variant="outlined" label="Active" />
  ) : (
    <Tooltip title="The page is on, but its own check hides it here (for example a feature flag or your access).">
      <Chip size="small" color="warning" variant="outlined" label="Hidden by its check" />
    </Tooltip>
  );
}

/** One-line summary of whether the module's pages are showing. */
export function pagesSummary(module: UIModuleStatus): string {
  if (module.installPhase === "not-installed") return "Not installed: its pages are hidden";
  if (module.installPhase === "installing") return "Installing: its pages are hidden until done";
  if (!module.enabled) return "Off: none of its pages load";
  const on = module.plugins.filter((p) => p.enabled).length;
  return `${on} of ${module.plugins.length} pages on`;
}

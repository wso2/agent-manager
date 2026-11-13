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

// Shared pieces for the module marketplace and module details pages.
//
// MOCK: install and uninstall only change in-memory state (PluginProvider);
// nothing is deployed. Listing details (icon, category, version, publisher)
// are mock data kept here until modules publish their own.

import { useCallback } from "react";
import {
  Activity,
  BookOpenText,
  Box as BoxIcon,
  IdCard,
  Network,
  Rocket,
} from "@wso2/oxygen-ui-icons-react";
import {
  Button,
  Chip,
  CircularProgress,
} from "@wso2/oxygen-ui";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import type { IconType } from "@agent-management-platform/types";
import { useUIModuleManager, type UIModuleStatus } from "../../plugins";

export interface MarketplaceListing {
  icon: IconType;
  category: string;
  publisher: string;
  version: string;
}

const MOCK_LISTINGS: Record<string, MarketplaceListing> = {
  "acp-core": { icon: BoxIcon, category: "Platform", publisher: "WSO2", version: "1.0.0" },
  "agent-runtime": { icon: Rocket, category: "Runtime", publisher: "WSO2", version: "1.0.0" },
  "ai-gateway": { icon: Network, category: "Gateway", publisher: "WSO2", version: "1.0.0" },
  "ai-observability": { icon: Activity, category: "Observability", publisher: "WSO2", version: "1.0.0" },
  "agent-identity": { icon: IdCard, category: "Security", publisher: "WSO2", version: "1.0.0" },
  "agent-portal": { icon: BookOpenText, category: "Catalog", publisher: "WSO2", version: "1.0.0" },
};

const FALLBACK_LISTING: MarketplaceListing = {
  icon: BoxIcon,
  category: "Extension",
  publisher: "Third party",
  version: "1.0.0",
};

export const listingFor = (moduleId: string) => MOCK_LISTINGS[moduleId] ?? FALLBACK_LISTING;

export function countLabel(count: number, singular: string) {
  return `${count} ${singular}${count === 1 ? "" : "s"}`;
}

/** Marketplace state of a module: what its card and details page show. */
export type MarketplaceState =
  | "built-in"
  | "installed"
  | "installing"
  | "available"
  | "checking"
  | "turned-off";

export function marketplaceState(module: UIModuleStatus): MarketplaceState {
  if (module.core) return "built-in";
  // Turned off by the deployment (uiModules.disabled): the Marketplace must not
  // switch it back on.
  if (!module.enabled) return "turned-off";
  if (module.installPhase === "installing") return "installing";
  if (module.installPhase === "checking") return "checking";
  if (module.installPhase === "installed") return "installed";
  return "available";
}

export function StatusChip({ state }: { state: MarketplaceState }) {
  switch (state) {
    case "built-in":
      return <Chip size="small" variant="outlined" label="Built in" />;
    case "installed":
      return <Chip size="small" color="success" variant="outlined" label="Installed" />;
    case "installing":
      return <Chip size="small" color="info" label="Installing" />;
    case "checking":
      return <Chip size="small" variant="outlined" label="Checking" />;
    case "turned-off":
      return <Chip size="small" variant="outlined" label="Turned off by admin" />;
    default:
      return null;
  }
}

/** How long the mock install "runs" before the module counts as installed. */
const MOCK_INSTALL_MS = 1500;

/** Mock install / uninstall. Only changes in-memory state; nothing is deployed. */
export function useModuleInstaller() {
  const { setInstallPhase } = useUIModuleManager();
  const { addConfirmation } = useConfirmationDialog();

  // Only the install phase changes: a module the deployment turned off stays
  // off (its card offers no Install).
  const install = useCallback(
    (module: UIModuleStatus) => {
      setInstallPhase(module.id, "installing");
      window.setTimeout(() => setInstallPhase(module.id, "installed"), MOCK_INSTALL_MS);
    },
    [setInstallPhase],
  );

  const uninstall = useCallback(
    (module: UIModuleStatus) =>
      addConfirmation({
        analytics: { entity: "ui-module", action: "uninstall" },
        title: `Uninstall ${module.name}?`,
        description: `Its pages and menu items are removed from the console. You can install it again from the marketplace.`,
        confirmButtonText: "Uninstall",
        confirmButtonColor: "error",
        onConfirm: () => setInstallPhase(module.id, "not-installed"),
      }),
    [addConfirmation, setInstallPhase],
  );

  return { install, uninstall };
}

/** The card's and details page's main action for a module. */
export function InstallButton({
  module,
  size,
}: {
  module: UIModuleStatus;
  size?: "small" | "medium";
}) {
  const { install, uninstall } = useModuleInstaller();
  const state = marketplaceState(module);
  if (state === "built-in" || state === "turned-off") return null;
  if (state === "installing" || state === "checking") {
    return (
      <Button
        size={size}
        variant="contained"
        disabled
        startIcon={<CircularProgress size={14} color="inherit" />}
      >
        {state === "installing" ? "Installing" : "Checking"}
      </Button>
    );
  }
  return state === "installed" ? (
    <Button size={size} variant="outlined" color="error" onClick={() => uninstall(module)}>
      Uninstall
    </Button>
  ) : (
    <Button size={size} variant="contained" onClick={() => install(module)}>
      Install
    </Button>
  );
}

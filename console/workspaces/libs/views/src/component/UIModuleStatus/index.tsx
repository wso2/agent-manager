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

import { createContext, useContext } from "react";
import type { UIModuleInstallPhase } from "@agent-management-platform/types";

/**
 * Install phase of each UI module, by module id, as the shell's module host sees
 * it. A module turned off by the deployment (public/ui-modules.js) reports
 * "not-installed". Lets a page in one module adapt to whether another is
 * present without importing the shell.
 */
export type UIModuleStatusMap = Record<string, UIModuleInstallPhase>;

// Null outside a module host (tests, stories): callers then assume installed,
// which is how the console behaved before modules could be absent.
const UIModuleStatusContext = createContext<UIModuleStatusMap | null>(null);

export const UIModuleStatusProvider = UIModuleStatusContext.Provider;

/**
 * Whether a UI module is installed: true, false, or undefined while its
 * detection is still running. A module someone is still installing counts as
 * not installed, as does an unknown module id; with no module host at all,
 * every module counts as installed.
 */
export function useIsUIModuleInstalled(moduleId: string): boolean | undefined {
  const phases = useContext(UIModuleStatusContext);
  if (!phases) return true;
  const phase = phases[moduleId];
  if (phase === "installed") return true;
  if (phase === "checking") return undefined;
  return false;
}

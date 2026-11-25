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

import { useCallback } from "react";
import { useLocation, useParams } from "react-router-dom";
import { useNavEntries } from "./useNavEntries";
import {
  currentNavLevel,
  isNavEntryActive,
  isNavEntryShownAt,
  navEntryHref,
  type NavParams,
} from "./navModel";

/**
 * Where to go when the top bar moves to another org, project or agent: the
 * same menu item at the new level (its page, or its gate there), so the
 * selection in the left menu doesn't change. Pages outside the menu (wizards,
 * profile) fall back to the given level home.
 */
export function useLevelSwitchHref() {
  const entries = useNavEntries();
  const { pathname } = useLocation();
  const { projectId, envId } = useParams();
  const active = entries.find((entry) => isNavEntryActive(entry, pathname));

  return useCallback(
    (target: NavParams, fallback: string) => {
      const level = currentNavLevel(target);
      // Not a menu page, or an item the new level doesn't list (picking a
      // project from the Projects list): go to the level's home.
      if (!active || !level || !isNavEntryShownAt(active, level)) {
        return fallback;
      }
      // Environments belong to the project: keep the one in view only within it.
      const keepEnv = !!target.projectId && target.projectId === projectId;
      return navEntryHref(active, level, {
        ...target,
        envId: keepEnv ? envId : undefined,
      });
    },
    [active, projectId, envId],
  );
}

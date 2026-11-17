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

import { Fragment, type ComponentType } from "react";
import { Grid } from "@wso2/oxygen-ui";
import {
  useExternalComponentModulesByMountPoint,
  type ExternalComponentModule,
} from "@agent-management-platform/views";

/** Consecutive half-width panels pair into one row; a lone one fills its row. */
function toRows(panels: ExternalComponentModule[]): ExternalComponentModule[][] {
  const rows: ExternalComponentModule[][] = [];
  for (let i = 0; i < panels.length; i++) {
    const next = panels[i + 1];
    if (panels[i].width === "half" && next?.width === "half") {
      rows.push([panels[i], next]);
      i++;
    } else {
      rows.push([panels[i]]);
    }
  }
  return rows;
}

/**
 * Renders the panels modules contribute at an overview mount point, in their
 * `order`, each with `props`. A panel that has nothing to show renders null;
 * to give up its half of a row instead, it should not be active (an
 * activation hook), so its partner takes the full row.
 */
export function OverviewPanels<P extends object>({
  mountPoint,
  props,
}: {
  mountPoint: string;
  props: P;
}) {
  const panels = useExternalComponentModulesByMountPoint(mountPoint);
  return (
    <>
      {toRows(panels).map((row) => {
        const render = (panel: ExternalComponentModule) => {
          const Panel = panel.component as unknown as ComponentType<P>;
          return <Panel {...props} />;
        };
        if (row.length === 1 && row[0].width !== "half") {
          return <Fragment key={row[0].moduleName}>{render(row[0])}</Fragment>;
        }
        return (
          <Grid key={row.map((p) => p.moduleName).join("+")} container spacing={2} sx={{ mb: 1.5 }}>
            {row.map((panel) => (
              <Grid key={panel.moduleName} size={{ xs: 12, md: row.length === 2 ? 6 : 12 }}>
                {render(panel)}
              </Grid>
            ))}
          </Grid>
        );
      })}
    </>
  );
}

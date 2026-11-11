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

// Module manager: an overview of the console's UI modules, whether each is
// installed and on, with a link to each module's setup page. Changes happen on
// the setup pages (ModuleSetup), not here.
//
// This page belongs to the shell, not to a module, so it can never switch
// itself off.

import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { PageLayout } from "@agent-management-platform/views";
import { useUIModuleManager, type UIModuleStatus } from "../../plugins";
import { ConfigChip, InstallChip, pagesSummary } from "./moduleUi";
import { MODULE_SETUP_ROUTE } from "./routes";

function ModuleRow({ module, onOpen }: { module: UIModuleStatus; onOpen: () => void }) {
  const needsSetup =
    !!module.installation && module.installPhase !== "installed" && !module.core;
  return (
    <Card variant="outlined">
      <CardContent>
        <Stack direction="row" alignItems="center" spacing={2}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Stack direction="row" alignItems="center" spacing={1}>
              <Typography variant="h6">{module.name}</Typography>
              <InstallChip module={module} />
              {!module.core && (
                <ConfigChip enabled={module.enabled} enabledByConfig={module.enabledByConfig} />
              )}
            </Stack>
            <Typography variant="body2" color="text.secondary">
              {module.description}
            </Typography>
            <Typography variant="caption" color="text.secondary">
              {module.id} · {pagesSummary(module)}
            </Typography>
          </Box>
          <Button variant={needsSetup ? "contained" : "outlined"} onClick={onOpen}>
            {needsSetup ? "Set up" : "Manage"}
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

  return (
    <PageLayout
      title="Module manager"
      description="See which console modules are installed and on. Open a module to set it up or change it."
      disableIcon
      actions={
        <Button variant="outlined" disabled={!hasChanges} onClick={resetChanges}>
          Reset to config
        </Button>
      }
    >
      <Stack spacing={2}>
        <Alert severity="info">
          Changes made on module setup pages apply straight away but only in this browser
          tab, and are lost when you reload. To make them permanent, set{" "}
          <code>uiModules.disabled</code> in the console&apos;s runtime config (the{" "}
          <code>DISABLED_UI_MODULES</code> environment variable).
        </Alert>
        {modules.map((module) => (
          <ModuleRow
            key={module.id}
            module={module}
            onOpen={() =>
              navigate(generatePath(MODULE_SETUP_ROUTE, { orgId: orgId ?? "", moduleId: module.id }))
            }
          />
        ))}
      </Stack>
    </PageLayout>
  );
}

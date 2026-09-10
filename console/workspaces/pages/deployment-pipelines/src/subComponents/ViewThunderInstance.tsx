/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */
import React, { useCallback } from "react";
import {
  Alert,
  Button,
  Card,
  Chip,
  Grid,
  Skeleton,
  Stack,
  Tooltip,
} from "@wso2/oxygen-ui";
import { AlertTriangle, CheckCircle, ExternalLink } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useParams } from "react-router-dom";
import { useListThunderInstances } from "@agent-management-platform/api-client";
import { absoluteRouteMap, globalConfig } from "@agent-management-platform/types";
import { copyToClipboard } from "@agent-management-platform/shared-component";
import { PageLayout, useSnackBar } from "@agent-management-platform/views";
import { ThunderInstanceOverviewTab } from "./ThunderInstanceOverviewTab";

export const ViewThunderInstance: React.FC = () => {
  const { orgId, envName } = useParams<{ orgId: string; envName: string }>();
  const { pushSnackBar } = useSnackBar();

  const { data, isLoading, error } = useListThunderInstances({ orgName: orgId });
  const instance = data?.thunderInstances.find((i) => i.envName === envName);

  const handleCopy = useCallback(
    (value: string, label: string) => {
      void copyToClipboard(value).then((succeeded) => {
        pushSnackBar(
          succeeded
            ? { message: `${label} copied to clipboard`, type: "success" }
            : { message: `Failed to copy ${label}`, type: "error" },
        );
      });
    },
    [pushSnackBar],
  );

  const backHref = generatePath(
    absoluteRouteMap.children.org.children.environments.children.view.path,
    { orgId: orgId ?? "", envName: envName ?? "" },
  );

  const consoleUrl = instance ? `${instance.issuerUrl.replace(/\/$/, "")}/console` : undefined;

  return (
      <PageLayout
        title="ThunderID"
        backHref={backHref}
        backLabel="Back to Environment"
        description={`The agent identity used to manage agents, users, roles, and groups for the ${envName} environment.`}
        disableIcon
        isLoading={isLoading}
        actions={
          consoleUrl && globalConfig.featureFlags?.disableThunderConsole !== true ? (
            <Button
              variant="outlined"
              size="small"
              startIcon={<ExternalLink size={16} />}
              component="a"
              href={consoleUrl}
              target="_blank"
              rel="noopener noreferrer"
            >
              Go to the Console
            </Button>
          ) : undefined
        }
        titleTail={
          instance && !error ? (
            <Tooltip title="Thunder identity provider is active for this environment">
              <Chip
                icon={<CheckCircle size={14} />}
                label="Active"
                size="small"
                color="success"
                variant="outlined"
              />
            </Tooltip>
          ) : undefined
        }
      >
        {isLoading && (
          <Stack spacing={3}>
            <Grid container spacing={2}>
              {[0, 1, 2, 3].map((i) => (
                <Grid key={i} size={{ xs: 12, sm: 6, md: 3 }}>
                  <Card variant="outlined" sx={{ p: 2, height: "100%" }}>
                    <Stack spacing={0.5}>
                      <Skeleton variant="text" width="40%" height={14} />
                      <Skeleton variant="text" width="85%" height={20} />
                    </Stack>
                  </Card>
                </Grid>
              ))}
            </Grid>
            <Card variant="outlined" sx={{ p: 3 }}>
              <Stack spacing={2}>
                <Skeleton variant="text" width={140} height={24} />
                <Skeleton variant="rounded" height={80} />
              </Stack>
            </Card>
          </Stack>
        )}

        {!!error && (
          <Alert severity="error" icon={<AlertTriangle size={18} />}>
            Failed to load agent identity. Please try again.
          </Alert>
        )}

        {!isLoading && !error && !instance && (
          <Alert severity="warning" icon={<AlertTriangle size={18} />}>
            Agent identity for environment &quot;{envName}&quot; was not found.
          </Alert>
        )}

        {instance && !error && (
          <Card variant="outlined" sx={{ p: 3 }}>
            <ThunderInstanceOverviewTab instance={instance} onCopy={handleCopy} />
          </Card>
        )}
      </PageLayout>
  );
};

export default ViewThunderInstance;

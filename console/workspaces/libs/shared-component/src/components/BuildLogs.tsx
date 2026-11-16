/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import {
  useGetBuild,
  useGetBuildLogs,
} from "@agent-management-platform/api-client";
import {
  DrawerHeader,
  DrawerContent,
  LogsPanel,
} from "@agent-management-platform/views";
import { Clock, FileText, Logs } from "@wso2/oxygen-ui-icons-react";
import { formatRelativeTime } from "../utils/format";
import { Alert, Box, Stack, Typography } from "@wso2/oxygen-ui";
import { BuildSteps, BuildStepsSkeleton } from "./BuildSteps";

import { getErrorMessage } from "../utils/errorHelpers";
import { useMemo, useState } from "react";

export interface BuildLogsProps {
  onClose: () => void;
  onFullscreenChange?: (fullscreen: boolean) => void;
  orgName: string;
  projName: string;
  agentName: string;
  buildName: string;
}

export function BuildLogs({
  buildName,
  orgName,
  projName,
  agentName,
  onClose,
  onFullscreenChange,
}: BuildLogsProps) {
  const [isFullscreen, setIsFullscreen] = useState(false);
  const handleToggleFullscreen = () => {
    const next = !isFullscreen;
    setIsFullscreen(next);
    onFullscreenChange?.(next);
  };
  const {
    data: build,
    isLoading: isBuildLoading,
    error: buildError,
  } = useGetBuild({
    orgName,
    projName,
    agentName,
    buildName,
  });

  const {
    data: buildLogs,
    error,
    isLoading,
  } = useGetBuildLogs(
    {
      orgName,
      projName,
      agentName,
      buildName,
    },
    build?.status
  );

  const reversedBuildLogs = useMemo(() => buildLogs ? [...buildLogs].reverse() : [], [buildLogs]);

  const getEmptyStateMessage = () => {
    if (error) {
      return {
        title: "Unable to Load Logs",
        subtitle:
          "There was an error retrieving the logs. Please try refreshing. If the issue persists, contact support.",
      };
    }

    if (build?.status === "Running" || build?.status === "Pending") {
      return {
        title: "Logs Being Generated",
        subtitle:
          "Build is in progress. Logs will appear shortly. Try refreshing in a few moments.",
      };
    }

    if (build?.status === "Failed") {
      return {
        title: "Unable to Retrieve Logs",
        subtitle:
          "The build logs could not be loaded. Please try refreshing or check back later.",
      };
    }

    return {
      title: "Logs Not Loaded",
      subtitle:
        "Build logs are not currently available. Please try refreshing the page. If the issue persists, there may be a temporary system issue.",
    };
  };

  const emptyState = getEmptyStateMessage();
  const logsEmptyState = {
    title: emptyState.title,
    description: emptyState.subtitle,
    illustration: <FileText size={64} />,
  };

  return (
    <Stack direction="column" height="100%">
      <DrawerHeader
        icon={<Logs size={24} />}
        title="Build Details"
        onClose={onClose}
        meta={
          build?.startedAt ? (
            <Stack
              direction="row"
              gap={0.75}
              alignItems="center"
              sx={{ flexShrink: 0, color: "text.secondary" }}
            >
              <Clock size={14} />
              <Typography variant="caption" noWrap>
                Triggered {formatRelativeTime(build.startedAt)}
              </Typography>
            </Stack>
          ) : undefined
        }
        isFullscreen={isFullscreen}
        onToggleFullscreen={handleToggleFullscreen}
      />
      <DrawerContent>
        {/* The drawer body owns the available height, so the panel flexes
            into it rather than guessing at viewport maths. */}
        <Stack direction="column" gap={2} sx={{ flex: 1, minHeight: 0 }}>
          {(!!buildError) && (
            <Alert severity="error">
              {getErrorMessage(buildError)}
            </Alert>
          )}
          {isBuildLoading && <BuildStepsSkeleton />}
          {build && <BuildSteps build={build} />}
          <Box sx={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
            <LogsPanel
              logs={reversedBuildLogs}
              isLoading={isLoading}
              error={error}
              showSearch={false}
              showTimestamp={false}
              showLogLevel={false}
              maxHeight="100%"
              emptyState={logsEmptyState}
            />
          </Box>
        </Stack>
      </DrawerContent>
    </Stack>
  );
}

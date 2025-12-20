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
  NoDataFound,
  DrawerHeader,
  DrawerContent,
} from "@agent-management-platform/views";
import { Clock, FileText, Logs } from "@wso2/oxygen-ui-icons-react";
import {
  Alert,
  Box,
  Divider,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import { BuildSteps } from "./BuildSteps";
import dayjs from "dayjs";

export interface BuildLogsProps {
  onClose: () => void;
  orgName: string;
  projName: string;
  agentName: string;
  buildName: string;
}

const InfoLoadingSkeleton = () => (
  <Box display="flex" flexDirection="column" gap={1}>
    <Skeleton variant="rounded" height={24} width={200} />
    <Skeleton variant="rounded" height={15} width={150} />
  </Box>
);

export function BuildLogs({
  buildName,
  orgName,
  projName,
  agentName,
  onClose,
}: BuildLogsProps) {
  const {
    data: buildLogs,
    error,
    isLoading,
  } = useGetBuildLogs({
    orgName,
    projName,
    agentName,
    buildName,
  });

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

  const getEmptyStateMessage = () => {
    if (error) {
      return {
        title: "Unable to Load Logs",
        subtitle:
          "There was an error retrieving the logs. Please try refreshing. If the issue persists, contact support.",
      };
    }

    if (
      build?.status === "BuildRunning" ||
      build?.status === "BuildTriggered"
    ) {
      return {
        title: "Logs Being Generated",
        subtitle:
          "Build is in progress. Logs will appear shortly. Try refreshing in a few moments.",
      };
    }

    if (build?.status === "BuildFailed") {
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

  return (
    <Stack direction="column" height="100%" maxWidth={900}>
      <DrawerHeader
        icon={<Logs size={24} />}
        title="Build Details"
        onClose={onClose}
      />
      <DrawerContent>
        <Stack direction="column" gap={2} height="calc(100vh - 72px)">
          {build?.startedAt && (
            <Stack direction="row" gap={1} alignItems="center">
              <Clock size={16} />
              <Typography variant="body2" color="text.secondary">
                Triggered {dayjs(build?.startedAt).fromNow()}
              </Typography>
            </Stack>
          )}
          {error && (
            <Alert severity="error">
              {error?.message ??
                "Failed to load build logs. Please try refreshing."}
            </Alert>
          )}
          {buildError && (
            <Alert severity="error">
              {buildError?.message ??
                "Failed to load build details. Please try refreshing."}
            </Alert>
          )}
          {isBuildLoading && <InfoLoadingSkeleton />}
          {build && <BuildSteps build={build} />}
          {!buildLogs?.length && !isLoading && !error && (
            <NoDataFound
              message={emptyState.title}
              subtitle={emptyState.subtitle}
              disableBackground
              iconElement={FileText}
            />
          )}
          {buildLogs?.length && <Divider />}
          <Stack direction="column" gap={1} overflow="auto" mb={1}>
            {buildLogs?.map((log, index) => (
              <Typography variant="body1" key={index}>
                {log.log}
              </Typography>
            ))}
          </Stack>
        </Stack>
      </DrawerContent>
    </Stack>
  );
}

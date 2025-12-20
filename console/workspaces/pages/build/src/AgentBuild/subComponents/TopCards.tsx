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

import { Box, CircularProgress, Skeleton } from "@wso2/oxygen-ui";
import { StatusCard } from "@agent-management-platform/views";
import {
  CheckCircle as CheckCircleIcon,
  XCircle as ErrorIcon,
  Play as PlayArrow,
  AlertTriangle as Warning,
} from "@wso2/oxygen-ui-icons-react";
import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
import relativeTime from "dayjs/plugin/relativeTime";
import { BuildStatus } from "@agent-management-platform/types";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";
dayjs.extend(duration);
dayjs.extend(relativeTime);
export interface TopCardsProps {
  buildCount: number;
  successfulBuildCount: number;
  latestBuildTime: number;
  latestBuildStatus: string;
  averageBuildTime: number;
}
const getBuildIcon = (status: BuildStatus) => {
  switch (status) {
    case "BuildCompleted":
    case "WorkloadUpdated":
      return <CheckCircleIcon size={20} />;
    case "BuildTriggered":
      return <PlayArrow size={20} />;
    case "BuildRunning":
      return <CircularProgress size={20} color="inherit" />;
    case "BuildFailed":
      return <ErrorIcon size={20} />;
    default:
      return <ErrorIcon size={20} />;
  }
};
const percIcon = (percentage: number) => {
  if (isNaN(percentage)) {
    return <CircularProgress size={20} color="inherit" />;
  }
  if (percentage >= 0.9) {
    return <CheckCircleIcon size={20} />;
  } else if (percentage >= 0.5) {
    return <Warning size={20} />;
  } else {
    return <ErrorIcon size={20} />;
  }
};
const percIconVariant = (percentage: number) => {
  if (isNaN(percentage)) {
    return "warning";
  }
  if (percentage >= 0.9) {
    return "success";
  } else if (percentage >= 0.5) {
    return "warning";
  } else {
    return "error";
  }
};

const getBuildIconVariant = (
  status: BuildStatus
): "success" | "warning" | "error" | "info" => {
  switch (status) {
    case "BuildCompleted":
    case "WorkloadUpdated":
      return "success"; // greenish for success
    case "BuildTriggered":
      return "warning";
    case "BuildRunning":
      return "warning";
    case "BuildFailed":
      return "error"; // red for failed
    default:
      return "info";
  }
};
const getTagVariant = (
  status: BuildStatus
): "success" | "warning" | "error" | "info" | "default" => {
  switch (status) {
    case "BuildCompleted":
    case "WorkloadUpdated":
      return "success";
    case "BuildTriggered":
      return "warning";
    case "BuildRunning":
      return "warning";
    case "BuildFailed":
      return "error";
    default:
      return "default";
  }
};

function TopCardsSkeleton() {
  return (
    <Box
      sx={{
        display: "grid",
        gap: 2,
        gridTemplateColumns: {
          xs: "1fr",
          md: "1fr 1fr",
          lg: "1fr 1fr 1fr",
        },
      }}
    >
      <Skeleton variant="rectangular" height={100} />
      <Skeleton variant="rectangular" height={100} />
      <Skeleton variant="rectangular" height={100} />
    </Box>
  );
}
export const TopCards: React.FC = () => {
  const { agentId, projectId, orgId } = useParams();
  const { data: builds, isLoading } = useGetAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  // Latest Build
  const latestBuild = builds?.builds[0];
  const latestBuildStatus = latestBuild?.status ?? "";
  const latestBuildStartedTime = latestBuild?.startedAt ?? "";

  // Summery
  const succesfullBuildCount =
    builds?.builds.filter(
      (build) =>
        build.status === "BuildCompleted" || build.status === "WorkloadUpdated"
    ).length ?? 0;
  const failedBuildCount =
    builds?.builds.filter((build) => build.status === "BuildFailed").length ??
    0;

  if (isLoading) {
    return <TopCardsSkeleton />;
  }

  return (
    <Box
      sx={{
        display: "grid",
        gap: 4,
        gridTemplateColumns: {
          xs: "1fr",
          md: "1fr 1fr",
          lg: "1fr 1fr 1fr",
        },
      }}
    >
      <StatusCard
        title="Latest Build"
        value={latestBuild?.status ?? ""}
        subtitle={dayjs(latestBuildStartedTime).fromNow()}
        icon={getBuildIcon(latestBuildStatus as BuildStatus)}
        iconVariant={getBuildIconVariant(latestBuildStatus as BuildStatus)}
        tagVariant={getTagVariant(latestBuildStatus as BuildStatus)}
        minWidth="100%"
      />
      <StatusCard
        title="Build Success Rate"
        value={`${((succesfullBuildCount / Math.max(1, succesfullBuildCount + failedBuildCount)) * 100).toFixed(2)}%`}
        subtitle="last 30 days"
        icon={percIcon(
          succesfullBuildCount / (succesfullBuildCount + failedBuildCount)
        )}
        iconVariant={percIconVariant(
          succesfullBuildCount / (succesfullBuildCount + failedBuildCount)
        )}
        tag={`${succesfullBuildCount}/${succesfullBuildCount + failedBuildCount}`}
        tagVariant={percIconVariant(
          succesfullBuildCount / (succesfullBuildCount + failedBuildCount)
        )}
        minWidth="100%"
      />
    </Box>
  );
};

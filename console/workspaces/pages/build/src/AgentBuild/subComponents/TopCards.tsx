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

import { Grid, Skeleton, StatCard, CircularProgress } from "@wso2/oxygen-ui";
import {
  CheckCircle,
  XCircle,
  TrendingUp,
} from "@wso2/oxygen-ui-icons-react";
import dayjs from "dayjs";
import duration from "dayjs/plugin/duration";
import relativeTime from "dayjs/plugin/relativeTime";
import { BuildStatus } from "@agent-management-platform/types";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";

dayjs.extend(duration);
dayjs.extend(relativeTime);

const getBuildIconColor = (
  status: BuildStatus
): "success" | "warning" | "error" | "info" => {
  switch (status) {
    case "Completed":
    case "Succeeded":
      return "success"; // greenish for success
    case "Pending":
      return "warning";
    case "Running":
      return "warning";
    case "Failed":
      return "error"; // red for failed
    default:
      return "info";
  }
};


const getSuccessRateColor = (
  percentage: number
): "primary" | "success" | "warning" | "error" | "info" | "secondary" => {
  if (isNaN(percentage)) return "warning";
  if (percentage >= 0.9) return "success";
  if (percentage >= 0.5) return "warning";
  return "error";
};

function TopCardsSkeleton() {
  return (
    <Grid container spacing={3}>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <Skeleton variant="rectangular" height={120} />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <Skeleton variant="rectangular" height={120} />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <Skeleton variant="rectangular" height={120} />
      </Grid>
    </Grid>
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
  // Summery
  const successfulBuildCount =
    builds?.builds.filter(
      (build) =>
        build.status === "Completed" || build.status === "Succeeded"
    ).length ?? 0;
  const failedBuildCount =
    builds?.builds.filter((build) => build.status === "Failed").length ??
    0;

  if (isLoading) {
    return <TopCardsSkeleton />;
  }

  const totalBuilds = builds?.builds.length ?? 0;
  const successRate = successfulBuildCount / Math.max(1, successfulBuildCount + failedBuildCount);
  
  const isLatestBuildRunning = latestBuildStatus === "Pending" || latestBuildStatus === "Running";
  const latestBuildIcon = !latestBuild ? (
    <XCircle size={24} />
  ) : isLatestBuildRunning ? (
    <CircularProgress size={24} />
  ) : latestBuildStatus === "Failed" ? (
    <XCircle size={24} />
  ) : (
    <CheckCircle size={24} />
  );

  return (
    <Grid container spacing={3}>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value={latestBuild?.status ?? "No builds"}
          label="Latest Build Status"
          icon={latestBuildIcon}
          iconColor={latestBuild ? getBuildIconColor(latestBuildStatus as BuildStatus) : "error"}
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value={`${(successRate * 100).toFixed(1)}%`}
          label="Build Success Rate"
          icon={<TrendingUp size={24} />}
          iconColor={getSuccessRateColor(successRate)}
        />
      </Grid>
      <Grid size={{ xs: 12, sm: 6, md: 4 }}>
        <StatCard
          value={totalBuilds.toString()}
          label="Total Builds"
          icon={<CheckCircle size={24} />}
          iconColor="primary"
        />
      </Grid>
    </Grid>
  );
};

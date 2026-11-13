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

import type { ReactNode } from "react";
import {
  Box,
  Card,
  CircularProgress,
  Grid,
  Skeleton,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";
import {
  CheckCircle,
  Hammer,
  TrendingUp,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import { IconTile, type IconTileColor } from "@agent-management-platform/views";
import { BuildStatus } from "@agent-management-platform/types";
import { useGetAllAgentBuilds } from "@agent-management-platform/api-client";
import { useParams } from "react-router-dom";

/** Card height, shared with the loading skeletons so nothing shifts on load. */
const CARD_HEIGHT = 76;
const GRID_ITEM_SIZE = { xs: 12, sm: 6, md: 4 } as const;

const getBuildStatusColor = (status: BuildStatus): IconTileColor => {
  switch (status) {
    case "Completed":
    case "Succeeded":
      return "success";
    case "Pending":
    case "Running":
      return "warning";
    case "Failed":
      return "error";
    default:
      return "info";
  }
};

const getSuccessRateColor = (rate: number): IconTileColor => {
  if (rate >= 0.9) return "success";
  if (rate >= 0.5) return "warning";
  return "error";
};

/**
 * A compact build stat: icon tile, label and value. Follows the organization
 * overview's resource cards (outlined card, tinted icon tile), without their
 * description and action footer.
 */
function BuildStatCard({
  icon,
  color,
  label,
  value,
}: {
  icon: ReactNode;
  color: IconTileColor;
  label: string;
  value: string;
}) {
  return (
    <Card variant="outlined" sx={{ height: CARD_HEIGHT }}>
      <Stack direction="row" spacing={2} alignItems="center" sx={{ p: 2, height: "100%" }}>
        <IconTile color={color} size={44}>
          {icon}
        </IconTile>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="body2" color="text.secondary" noWrap>
            {label}
          </Typography>
          <Typography variant="h6" component="p" sx={{ fontWeight: 600 }} noWrap>
            {value}
          </Typography>
        </Box>
      </Stack>
    </Card>
  );
}

function TopCardsSkeleton() {
  return (
    <Grid container spacing={2}>
      {Array.from({ length: 3 }).map((_, i) => (
        <Grid key={i} size={GRID_ITEM_SIZE}>
          <Skeleton variant="rounded" height={CARD_HEIGHT} />
        </Grid>
      ))}
    </Grid>
  );
}

export const TopCards: React.FC = () => {
  const { agentId, projectId, orgId } = useParams();
  const { data: builds, isLoading } = useGetAllAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  if (isLoading) {
    return <TopCardsSkeleton />;
  }

  const latestBuild = builds?.builds[0];
  const latestBuildStatus = (latestBuild?.status ?? "") as BuildStatus;
  const successfulBuildCount =
    builds?.builds.filter(
      (build) => build.status === "Completed" || build.status === "Succeeded",
    ).length ?? 0;
  const failedBuildCount =
    builds?.builds.filter((build) => build.status === "Failed").length ?? 0;
  const totalBuilds = builds?.builds.length ?? 0;
  const successRate =
    successfulBuildCount / Math.max(1, successfulBuildCount + failedBuildCount);

  const isLatestBuildRunning =
    latestBuildStatus === "Pending" || latestBuildStatus === "Running";
  const latestBuildIcon = !latestBuild ? (
    <XCircle size={20} />
  ) : isLatestBuildRunning ? (
    <CircularProgress size={18} color="inherit" />
  ) : latestBuildStatus === "Failed" ? (
    <XCircle size={20} />
  ) : (
    <CheckCircle size={20} />
  );

  return (
    <Grid container spacing={2}>
      <Grid size={GRID_ITEM_SIZE}>
        <BuildStatCard
          icon={latestBuildIcon}
          color={latestBuild ? getBuildStatusColor(latestBuildStatus) : "error"}
          label="Latest build status"
          value={latestBuild?.status ?? "No builds"}
        />
      </Grid>
      <Grid size={GRID_ITEM_SIZE}>
        <BuildStatCard
          icon={<TrendingUp size={20} />}
          color={getSuccessRateColor(successRate)}
          label="Build success rate"
          value={`${(successRate * 100).toFixed(1)}%`}
        />
      </Grid>
      <Grid size={GRID_ITEM_SIZE}>
        <BuildStatCard
          icon={<Hammer size={20} />}
          color="primary"
          label="Total builds"
          value={totalBuilds.toString()}
        />
      </Grid>
    </Grid>
  );
};

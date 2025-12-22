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
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  CircularProgress,
  Divider,
  Stack,
} from "@wso2/oxygen-ui";
import { useParams, useSearchParams } from "react-router-dom";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";
import { useMemo, useCallback, useEffect } from "react";
import {
  Clock as AccessTime,
  Edit,
  GitCommit,
  Rocket,
} from "@wso2/oxygen-ui-icons-react";
import { DeploymentConfig } from "@agent-management-platform/shared-component";
import { DrawerWrapper, NoDataFound } from "@agent-management-platform/views";
import { BuildSelectorDrawer } from "./BuildSelectorDrawer";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { Environment } from "@agent-management-platform/types";

dayjs.extend(relativeTime);

interface BuildCardProps {
  initialEnvironment?: Environment;
}
export function BuildCard(props: BuildCardProps) {
  const { initialEnvironment } = props;
  const { orgId, projectId, agentId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const { data: builds, isLoading: isBuildsLoading } = useGetAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  // Sort builds by most recent first
  const orderedBuilds = useMemo(
    () =>
      builds?.builds
        .sort(
          (a, b) =>
            new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
        )
        .filter(
          (build) =>
            build.status === "BuildCompleted" ||
            build.status === "WorkloadUpdated"
        ),
    [builds]
  );

  const selectedBuildFromParams = searchParams.get("selectedBuild");
  const isDrawerOpen = searchParams.get("deployPanel") === "open";
  const isBuildSelectorOpen = searchParams.get("buildSelector") === "open";

  // Set default selected build to the latest one if not in params
  useEffect(() => {
    if (!selectedBuildFromParams && orderedBuilds && orderedBuilds.length > 0) {
      const next = new URLSearchParams(searchParams);
      next.set("selectedBuild", orderedBuilds[0].buildName);
      setSearchParams(next, { replace: true });
    }
  }, [selectedBuildFromParams, orderedBuilds, searchParams, setSearchParams]);

  // Get selected build from params or default to latest
  const selectedBuild =
    selectedBuildFromParams ||
    (orderedBuilds && orderedBuilds.length > 0
      ? orderedBuilds[0].buildName
      : "");

  const currentBuild = orderedBuilds?.find(
    (build) => build.buildName === selectedBuild
  );

  const handleBuildChange = useCallback(
    (buildName: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("selectedBuild", buildName);
      next.delete("buildSelector");
      setSearchParams(next);
    },
    [searchParams, setSearchParams]
  );

  const handleOpenDeployment = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set("deployPanel", "open");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleCloseDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete("deployPanel");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleOpenBuildSelector = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set("buildSelector", "open");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleCloseBuildSelector = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete("buildSelector");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  if (isBuildsLoading) {
    return (
      <Card
        variant="outlined"
        sx={{
          "& .MuiCardContent-root": {
            backgroundColor: "background.paper",
          },
          width: 350,
          minWidth: 350,
          height: "fit-content",
        }}
      >
        <CardContent>
          <Box p={2} display="flex" justifyContent="center" alignItems="center">
            <CircularProgress />
          </Box>
        </CardContent>
      </Card>
    );
  }

  if (!orderedBuilds || orderedBuilds.length === 0) {
    return (
      <Card
        variant="outlined"
        sx={{
          "& .MuiCardContent-root": {
            backgroundColor: "background.paper",
          },
          height: "fit-content",
          width: 350,
          minWidth: 350,
        }}
      >
        <CardContent>
          <Stack gap={2} alignItems="center">
            <NoDataFound
              message="No builds available"
              icon={<Rocket size={32} />}
              disableBackground
            />
          </Stack>
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <Card
        variant="outlined"
        sx={{
          "& .MuiCardContent-root": {
            backgroundColor: "background.paper",
          },
          height: "fit-content",
          width: 350,
          minWidth: 350,
        }}
      >
        <CardContent>
          <Stack direction="column" gap={2}>
            <Typography variant="h4">Set up</Typography>
            <Divider />
            {/* Build ID Selector */}

            <Typography variant="body2" color="text.secondary">
              Select Build
            </Typography>

            <Button
              variant="outlined"
              fullWidth
              onClick={handleOpenBuildSelector}
              sx={{
                borderRadius: 0.5,
                justifyContent: "space-between",
                textTransform: "none",
              }}
            >
              <Stack gap={0.5} alignItems="flex-start">
                <Typography variant="body1">
                  {currentBuild?.buildName || "Select a build"}
                </Typography>
                {currentBuild && (
                  <Box display="flex" gap={1} sx={{ opacity: 0.7 }}>
                    <Box display="flex" alignItems="center" gap={0.5}>
                      <GitCommit size={16} />
                      <Typography variant="caption">
                        {currentBuild.commitId?.substring(0, 8) || "N/A"}
                      </Typography>
                    </Box>
                    <Box display="flex" alignItems="center" gap={0.5}>
                      <AccessTime size={12} />
                      <Typography variant="caption">
                        {dayjs(currentBuild.startedAt).format("DD MMM YYYY")}
                      </Typography>
                    </Box>
                  </Box>
                )}
              </Stack>
              <Edit size={16} />
            </Button>

            <Divider />
            {/* Selected Build Details */}
            <Button
              variant="contained"
              color="primary"
              fullWidth
              onClick={handleOpenDeployment}
              disabled={
                !currentBuild ||
                (currentBuild.status !== "BuildCompleted" &&
                  currentBuild.status !== "WorkloadUpdated")
              }
              startIcon={<Rocket size={16} />}
            >
              Configure & Deploy
            </Button>
          </Stack>
        </CardContent>
      </Card>
      {/* Build Selector Drawer */}
      <BuildSelectorDrawer
        open={isBuildSelectorOpen}
        onClose={handleCloseBuildSelector}
        builds={orderedBuilds || []}
        selectedBuild={selectedBuild}
        onSelectBuild={handleBuildChange}
      />

      {/* Deployment Drawer */}
      <DrawerWrapper open={isDrawerOpen} onClose={handleCloseDrawer}>
        {currentBuild && (
          <DeploymentConfig
            onClose={handleCloseDrawer}
            imageId={currentBuild.imageId || "busybox"}
            to={initialEnvironment?.name || "development"}
            orgName={orgId || ""}
            projName={projectId || ""}
            agentName={agentId || ""}
          />
        )}
      </DrawerWrapper>
    </>
  );
}

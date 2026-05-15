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
import {
  useGetAgent,
  useGetAgentBuilds,
  useGetAgentKind,
} from "@agent-management-platform/api-client";
import { useMemo, useCallback, useEffect } from "react";
import {
  Clock as AccessTime,
  Edit,
  GitCommit,
  Rocket,
  Tag,
} from "@wso2/oxygen-ui-icons-react";
import { DeploymentConfig } from "@agent-management-platform/shared-component";
import { DrawerWrapper, NoDataFound } from "@agent-management-platform/views";
import { BuildSelectorDrawer } from "./BuildSelectorDrawer";
import { KindVersionSelectorDrawer } from "./KindVersionSelectorDrawer";
import { format } from "date-fns";
import { Environment } from "@agent-management-platform/types";

interface BuildCardProps {
  initialEnvironment?: Environment;
}
export function BuildCard(props: BuildCardProps) {
  const { initialEnvironment } = props;
  const { orgId, projectId, agentId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const { data: agent, isLoading: isAgentLoading } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const isKindAgent = !!agent?.fromKind;

  // ── Build-agent data ────────────────────────────────────────────────────────
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
            build.status === "Completed" || build.status === "Succeeded"
        ),
    [builds]
  );

  const selectedBuildFromParams = searchParams.get("selectedBuild");
  const isBuildSelectorOpen = searchParams.get("buildSelector") === "open";

  // Set default selected build to the latest one if not in params
  useEffect(() => {
    if (
      !isKindAgent &&
      !selectedBuildFromParams &&
      orderedBuilds &&
      orderedBuilds.length > 0
    ) {
      const next = new URLSearchParams(searchParams);
      next.set("selectedBuild", orderedBuilds[0].buildName);
      setSearchParams(next, { replace: true });
    }
  }, [isKindAgent, selectedBuildFromParams, orderedBuilds, searchParams, setSearchParams]);

  const selectedBuild =
    selectedBuildFromParams ||
    (orderedBuilds && orderedBuilds.length > 0
      ? orderedBuilds[0].buildName
      : "");

  const currentBuild = orderedBuilds?.find(
    (build) => build.buildName === selectedBuild
  );

  // ── Kind-agent data ─────────────────────────────────────────────────────────
  const { data: kind, isLoading: isKindLoading } = useGetAgentKind({
    orgName: orgId,
    kindName: agent?.fromKind?.kindName  ?? "",
  });

  const sortedKindVersions = useMemo(
    () =>
      [...(kind?.versions ?? [])].sort(
        (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
      ),
    [kind]
  );

  const selectedVersionFromParams = searchParams.get("selectedVersion");
  const isVersionSelectorOpen = searchParams.get("versionSelector") === "open";

  // Default selected version to the agent's current kind version
  useEffect(() => {
    if (isKindAgent && !selectedVersionFromParams && agent?.fromKind?.version) {
      const next = new URLSearchParams(searchParams);
      next.set("selectedVersion", agent.fromKind.version);
      setSearchParams(next, { replace: true });
    }
  }, [isKindAgent, selectedVersionFromParams, agent, searchParams, setSearchParams]);

  const selectedVersion =
    selectedVersionFromParams ||
    agent?.fromKind?.version ||
    (sortedKindVersions.length > 0 ? sortedKindVersions[0].version : "");

  const currentKindVersion = sortedKindVersions.find(
    (v) => v.version === selectedVersion
  );

  // ── Shared handlers ─────────────────────────────────────────────────────────
  const isDrawerOpen = searchParams.get("deployPanel") === "open";

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

  // ── Build-agent handlers ────────────────────────────────────────────────────
  const handleBuildChange = useCallback(
    (buildName: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("selectedBuild", buildName);
      next.delete("buildSelector");
      setSearchParams(next);
    },
    [searchParams, setSearchParams]
  );

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

  // ── Kind-agent handlers ─────────────────────────────────────────────────────
  const handleVersionChange = useCallback(
    (version: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("selectedVersion", version);
      next.delete("versionSelector");
      setSearchParams(next);
    },
    [searchParams, setSearchParams]
  );

  const handleOpenVersionSelector = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set("versionSelector", "open");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleCloseVersionSelector = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete("versionSelector");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  // ── Loading state ───────────────────────────────────────────────────────────
  if (isAgentLoading || (!isKindAgent && isBuildsLoading) || (isKindAgent && isKindLoading)) {
    return (
      <Card
        variant="outlined"
        sx={{
          height: "fit-content",
          width: 350,
          minWidth: 350,
        }}
      >
        <CardContent>
          <Box p={8} display="flex" justifyContent="center" alignItems="center">
            <CircularProgress />
          </Box>
        </CardContent>
      </Card>
    );
  }

  // ── Kind agent branch ───────────────────────────────────────────────────────
  if (isKindAgent) {
    if (sortedKindVersions.length === 0) {
      return (
        <Card
          variant="outlined"
          sx={{ height: "fit-content", width: 350, minWidth: 350 }}
        >
          <CardContent>
            <Stack gap={2} alignItems="center">
              <NoDataFound
                message="No versions available"
                subtitle="Publish a version for this Agent Kind first."
                icon={<Tag size={32} />}
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
          sx={{ height: "fit-content", width: 350, minWidth: 350 }}
        >
          <CardContent>
            <Stack direction="column" gap={2}>
              <Typography variant="h5">Setup</Typography>
              <Divider />

              <Typography variant="body2" color="text.secondary">
                Select Kind Version
              </Typography>

              <Button
                variant="outlined"
                fullWidth
                onClick={handleOpenVersionSelector}
                sx={{
                  borderRadius: 0.5,
                  justifyContent: "space-between",
                  textTransform: "none",
                }}
              >
                <Stack gap={0.5} alignItems="flex-start">
                  <Typography variant="body1">
                    {currentKindVersion?.version || "Select a version"}
                  </Typography>
                  {currentKindVersion && (
                    <Box display="flex" gap={1} sx={{ opacity: 0.7 }}>
                      <Box display="flex" alignItems="center" gap={0.5}>
                        <AccessTime size={12} />
                        <Typography variant="caption">
                          {format(
                            new Date(currentKindVersion.createdAt),
                            "dd MMM yyyy"
                          )}
                        </Typography>
                      </Box>
                    </Box>
                  )}
                </Stack>
                <Edit size={16} />
              </Button>

              <Divider />

              <Button
                variant="contained"
                color="primary"
                fullWidth
                onClick={handleOpenDeployment}
                disabled={!currentKindVersion}
                startIcon={<Rocket size={16} />}
              >
                Configure & Deploy
              </Button>
            </Stack>
          </CardContent>
        </Card>

        {/* Kind Version Selector Drawer */}
        <KindVersionSelectorDrawer
          open={isVersionSelectorOpen}
          onClose={handleCloseVersionSelector}
          versions={sortedKindVersions}
          selectedVersion={selectedVersion}
          onSelectVersion={handleVersionChange}
        />

        {/* Deployment Drawer */}
        <DrawerWrapper open={isDrawerOpen} onClose={handleCloseDrawer}>
          {currentKindVersion && (
            <DeploymentConfig
              onClose={handleCloseDrawer}
              imageId={currentKindVersion.imageId}
              to={initialEnvironment?.name || "development"}
              orgName={orgId || ""}
              projName={projectId || ""}
              agentName={agentId || ""}
              configSchema={currentKindVersion.configSchema}
            />
          )}
        </DrawerWrapper>
      </>
    );
  }

  // ── Build agent branch ───────────────────────────────────────────────────────
  if (!orderedBuilds || orderedBuilds.length === 0) {
    return (
      <Card
        variant="outlined"
        sx={{
          height: "fit-content",
          width: 350,
          minWidth: 350,
        }}
      >
        <CardContent>
          <Stack gap={2} alignItems="center">
            <NoDataFound
              message="No builds available"
              subtitle={`build your agent first to deploy it to an environment.`}
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
          height: "fit-content",
          width: 350,
          minWidth: 350,
        }}
      >
        <CardContent>
          <Stack direction="column" gap={2}>
            <Typography variant="h5">Setup</Typography>
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
                        {currentBuild.buildParameters?.commitId?.substring(0, 8) || "N/A"}
                      </Typography>
                    </Box>
                    <Box display="flex" alignItems="center" gap={0.5}>
                      <AccessTime size={12} />
                      <Typography variant="caption">
                        {format(new Date(currentBuild.startedAt), "dd MMM yyyy")}
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
                (currentBuild.status !== "Completed" &&
                  currentBuild.status !== "Succeeded")
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

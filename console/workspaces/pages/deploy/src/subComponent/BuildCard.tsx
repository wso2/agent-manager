import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  CircularProgress,
  Divider,
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
import { DrawerWrapper } from "@agent-management-platform/views";
import { BuildSelectorDrawer } from "./BuildSelectorDrawer";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";

dayjs.extend(relativeTime);

export function BuildCard() {
  const { orgId, projectId, agentId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const { data: builds, isLoading: isBuildsLoading } = useGetAgentBuilds({
    orgName: orgId ?? "default",
    projName: projectId ?? "default",
    agentName: agentId ?? "",
  });

  // Sort builds by most recent first
  const orderedBuilds = useMemo(
    () =>
      builds?.builds
        .sort(
          (a, b) =>
            new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
        )
        .filter((build) => build.status === "Completed"),
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
            gap: 2,
            display: "flex",
            height: "100%",
            width: 350,
            minHeight: 200,
            justifyContent: "center",
            alignItems: "center",
            flexDirection: "column",
          },
        }}
      >
        <CardContent>
          <CircularProgress />
        </CardContent>
      </Card>
    );
  }

  if (!orderedBuilds || orderedBuilds.length === 0) {
    return (
      <Card
        variant="outlined"
        sx={{
          backgroundColor: "background.paper",
          width: 350,
          height: "100%",
        }}
      >
        <CardContent>
          <Box
            display="flex"
            justifyContent="center"
            alignItems="center"
            minHeight="200px"
          >
            <Typography variant="body1" color="text.secondary">
              No builds available
            </Typography>
          </Box>
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
          "& .MuiCardContent-root": {
            backgroundColor: "background.paper",
            gap: 2,
            display: "flex",
            flexDirection: "column",
            width: 350,
          },
        }}
      >
        <CardContent
          sx={{
            gap: 1,
            display: "flex",
            flexDirection: "column",
            justifyContent: "space-between",
          }}
        >
          <Box display="flex" flexDirection="column" gap={2}>
            <Typography variant="h4">Set up</Typography>
            <Divider />
          </Box>
          {/* Build ID Selector */}
          <Box display="flex" flexDirection="column" gap={1}>
            <Typography variant="body2" color="text.secondary">
              Select Build
            </Typography>

            <Button
              variant="outlined"
              fullWidth
              color="inherit"
              onClick={handleOpenBuildSelector}
              sx={{
                borderRadius: 0.5,
                justifyContent: "space-between",
                textTransform: "none",
              }}
            >
              <Box
                display="flex"
                flexDirection="column"
                alignItems="flex-start"
                gap={0.5}
              >
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
              </Box>
              <Edit size={16} />
            </Button>
          </Box>
          <Divider />
          {/* Selected Build Details */}
          <Button
            variant="contained"
            color="primary"
            fullWidth
            onClick={handleOpenDeployment}
            disabled={!currentBuild || currentBuild.status !== "Completed"}
            startIcon={<Rocket size={16} />}
          >
            Configure & Deploy
          </Button>
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
            to="development"
            orgName={orgId || ""}
            projName={projectId || ""}
            agentName={agentId || ""}
          />
        )}
      </DrawerWrapper>
    </>
  );
}

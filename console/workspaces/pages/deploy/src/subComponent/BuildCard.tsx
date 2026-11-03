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
  ButtonBase,
  Card,
  CardContent,
  Chip,
  IconButton,
  Tooltip,
  Typography,
  Button,
  CircularProgress,
  Stack,
} from "@wso2/oxygen-ui";
import { generatePath, Link, useParams, useSearchParams } from "react-router-dom";
import {
  useGetAgent,
  useGetAllAgentBuilds,
  useGetAgentKind,
  useListAgentDeployments,
} from "@agent-management-platform/api-client";
import React, { useMemo, useCallback, useEffect, useState } from "react";
import {
  ChevronDown,
  Clock as AccessTime,
  Copy,
  GitCommit,
  History,
  Hammer,
  Rocket,
  Tag,
} from "@wso2/oxygen-ui-icons-react";
import { StateNotice } from "@agent-management-platform/views";
import { EditDeployConfigDrawer } from "./EditDeployConfigDrawer";
import { BuildSelectorDrawer } from "./BuildSelectorDrawer";
import { KindVersionSelectorDrawer } from "./KindVersionSelectorDrawer";
import { format, formatDistanceToNow } from "date-fns";
import { absoluteRouteMap, Environment } from "@agent-management-platform/types";
import {
  RestrictedAction,
  useAgentEnvironmentAccess,
} from "@agent-management-platform/shared-component";

interface PickerRowProps {
  caption: string;
  title: string;
  ariaLabel: string;
  /** When set, shows a copy button that copies the title. */
  copyLabel?: string;
  startedAt?: string;
  latest?: boolean;
  onOpen: () => void;
  children?: React.ReactNode;
}

/** The neutral, bordered row that opens a build or version picker. */
function PickerRow({
  caption,
  title,
  ariaLabel,
  copyLabel,
  startedAt,
  latest,
  onOpen,
  children,
}: PickerRowProps) {
  const [copied, setCopied] = useState(false);
  const date = startedAt ? new Date(startedAt) : undefined;

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard?.writeText(title).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  };

  return (
    <Stack
      sx={{ border: 1, borderColor: "divider", borderRadius: 1, overflow: "hidden" }}
    >
      <ButtonBase
        component="div"
        role="button"
        tabIndex={0}
        aria-haspopup="dialog"
        aria-label={ariaLabel}
        onClick={onOpen}
        sx={{
          display: "flex",
          flexDirection: "column",
          alignItems: "stretch",
          gap: 0.5,
          width: "100%",
          px: 1.5,
          py: 1.5,
          textAlign: "left",
          "&:hover": { bgcolor: "action.hover" },
          "&:focus-visible": { outline: "2px solid", outlineColor: "primary.main", outlineOffset: -2 },
        }}
      >
        <Stack direction="row" alignItems="center" justifyContent="space-between" minHeight={24}>
          <Typography variant="overline" color="text.secondary" lineHeight={1.5}>
            {caption}
          </Typography>
          {latest && <Chip size="small" color="success" variant="outlined" label="Latest" />}
        </Stack>
        <Stack direction="row" alignItems="center" gap={0.5}>
          <Typography
            variant="body1"
            sx={{ flex: 1, minWidth: 0, overflowWrap: "anywhere", fontFamily: "monospace" }}
          >
            {title}
          </Typography>
          {copyLabel && (
            <Tooltip title={copied ? "Copied" : copyLabel}>
              <IconButton size="small" aria-label={copyLabel} onClick={handleCopy}>
                <Copy size={16} />
              </IconButton>
            </Tooltip>
          )}
          <ChevronDown size={16} />
        </Stack>
      </ButtonBase>
      {(date || children) && (
        <Stack
          direction="row"
          alignItems="center"
          justifyContent="space-between"
          gap={1}
          sx={{ px: 1.5, py: 1, borderTop: 1, borderColor: "divider" }}
        >
          {date && (
            <Tooltip title={format(date, "dd MMM yyyy")}>
              <Stack direction="row" alignItems="center" gap={0.5} color="text.secondary">
                <AccessTime size={14} />
                <Typography variant="caption">
                  {formatDistanceToNow(date, { addSuffix: true })}
                </Typography>
              </Stack>
            </Tooltip>
          )}
          {children}
        </Stack>
      )}
    </Stack>
  );
}

interface BuildCardProps {
  initialEnvironment?: Environment;
}
export function BuildCard(props: BuildCardProps) {
  const { initialEnvironment } = props;
  const { orgId, projectId, agentId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  // A deploy always lands in the pipeline's lowest environment, which is what
  // initialEnvironment is, so that is the environment whose tier decides it.
  // The tier is the whole check here: unlike the deployment-state route, deploy
  // declares no capability scope beside it.
  const environmentAccess = useAgentEnvironmentAccess(orgId);
  const deployAccess = environmentAccess(initialEnvironment);

  const { data: agent, isLoading: isAgentLoading } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const isKindAgent = !!agent?.kindName;
  const agentRoutes = absoluteRouteMap.children.org.children.projects.children.agents;
  const routeParams = { orgId: orgId ?? "", projectId: projectId ?? "", agentId: agentId ?? "" };
  const buildPath = generatePath(agentRoutes.children.build.path, routeParams);
  const publishPath = generatePath(agentRoutes.children.publish.path, routeParams);

  // ── Build-agent data ────────────────────────────────────────────────────────
  const { data: builds, isLoading: isBuildsLoading } = useGetAllAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  // Sort builds by most recent first
  const orderedBuilds = useMemo(
    () =>
      builds?.builds
        .slice()
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
    kindName: agent?.kindName  ?? "",
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

  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });

  // What the card opens on, in order: the version this environment already runs,
  // then the one the agent was created from, then the kind's newest. Defaulting to
  // newest — as this did — turns "Configure & Deploy" into an unannounced upgrade
  // for anyone who just wanted to redeploy, and shows a version the agent has
  // never run. Only versions the kind still publishes are offered, since a
  // deleted one cannot be deployed.
  const defaultKindVersion = useMemo(() => {
    const published = (v?: string) =>
      v && sortedKindVersions.some((kv) => kv.version === v) ? v : undefined;
    return (
      published(deployments?.[initialEnvironment?.name ?? ""]?.kindVersion) ??
      published(agent?.kindVersion) ??
      sortedKindVersions[0]?.version ??
      ""
    );
  }, [deployments, initialEnvironment?.name, agent?.kindVersion, sortedKindVersions]);

  useEffect(() => {
    // Writing the param pins the selection — the effect cannot correct it later,
    // because selectedVersionFromParams is set from then on. So wait for the
    // deployments query: pinning first would lock the card onto the fallback
    // (the creation version, or the kind's newest) even though this environment
    // runs something else.
    if (isDeploymentsLoading) return;
    if (isKindAgent && !selectedVersionFromParams && defaultKindVersion) {
      const next = new URLSearchParams(searchParams);
      next.set("selectedVersion", defaultKindVersion);
      setSearchParams(next, { replace: true });
    }
  }, [
    isDeploymentsLoading,
    isKindAgent,
    selectedVersionFromParams,
    defaultKindVersion,
    searchParams,
    setSearchParams,
  ]);

  const selectedVersion = selectedVersionFromParams || defaultKindVersion;

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
            <StateNotice
              compact
              icon={Tag}
              title="Publish a version first"
              description="Kind agents deploy from a published version, and this kind has none yet."
              steps={[
                { label: "Publish", state: "current" },
                { label: "Deploy", state: "todo" },
                { label: "Try it", state: "todo" },
              ]}
              actions={
                <Button variant="contained" component={Link} to={publishPath}>
                  Go to Publish
                </Button>
              }
            />
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
              <Typography variant="h5">Version</Typography>

              <PickerRow
                caption="Version"
                title={currentKindVersion?.version || "Select a version"}
                ariaLabel="Select a version"
                startedAt={currentKindVersion?.createdAt}
                latest={
                  !!currentKindVersion &&
                  currentKindVersion.version === sortedKindVersions[0]?.version
                }
                onOpen={handleOpenVersionSelector}
              />

              <RestrictedAction decision={deployAccess} feature="deploy-agent">
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
              </RestrictedAction>
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
        {currentKindVersion && initialEnvironment && (
          <EditDeployConfigDrawer
            open={isDrawerOpen}
            onClose={handleCloseDrawer}
            mode="deploy"
            imageId={currentKindVersion.imageId}
            orgName={orgId || ""}
            projName={projectId || ""}
            agentName={agentId || ""}
            environment={initialEnvironment.name}
            title={`Configure and Deploy to ${initialEnvironment.displayName ?? initialEnvironment.name} Environment`}
          />
        )}
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
          <StateNotice
            compact
            icon={Hammer}
            title="Build your agent first"
            description="There's no successful build to deploy yet. Build it, then deploy it here."
            steps={[
              { label: "Build", state: "current" },
              { label: "Deploy", state: "todo" },
              { label: "Try it", state: "todo" },
            ]}
            actions={
              <Button variant="contained" component={Link} to={buildPath}>
                Go to Build
              </Button>
            }
          />
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
            <Stack direction="row" alignItems="center" justifyContent="space-between">
              <Typography variant="h5">Setup</Typography>
              <Button
                size="small"
                variant="text"
                component={Link}
                to={buildPath}
                color="inherit"
                startIcon={<History size={16} />}
              >
                All builds
              </Button>
            </Stack>

            <PickerRow
              caption="Build ID"
              title={currentBuild?.buildName || "Select a build"}
              ariaLabel="Select a build"
              copyLabel={currentBuild ? "Copy build ID" : undefined}
              startedAt={currentBuild?.startedAt}
              latest={!!currentBuild && currentBuild.buildName === orderedBuilds?.[0]?.buildName}
              onOpen={handleOpenBuildSelector}
            >
              {currentBuild && (
                <Tooltip title="Commit">
                  <Chip
                    size="small"
                    variant="outlined"
                    icon={<GitCommit size={14} />}
                    label={currentBuild.buildParameters?.commitId?.substring(0, 8) || "N/A"}
                    sx={{ fontFamily: "monospace" }}
                  />
                </Tooltip>
              )}
            </PickerRow>

            {/* Selected Build Details */}
            <RestrictedAction decision={deployAccess} feature="deploy-agent">
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
            </RestrictedAction>
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
      {currentBuild?.imageId && initialEnvironment && (
        <EditDeployConfigDrawer
          open={isDrawerOpen}
          onClose={handleCloseDrawer}
          mode="deploy"
          imageId={currentBuild.imageId}
          orgName={orgId || ""}
          projName={projectId || ""}
          agentName={agentId || ""}
          environment={initialEnvironment.name}
          title={`Configure and Deploy to ${initialEnvironment.displayName ?? initialEnvironment.name} Environment`}
        />
      )}
    </>
  );
}

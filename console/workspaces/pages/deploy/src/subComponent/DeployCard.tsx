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
  useGetAgent,
  useGetAgentConfigurations,
  useGetAgentMetrics,
  useGetAgentResourceConfigs,
  useGetDeploymentPipeline,
  useListAgentDeployments,
  useListAgentKindVersions,
  useUpdateDeploymentState,
} from "@agent-management-platform/api-client";
import { NoDataFound, TextInput } from "@agent-management-platform/views";
import {
  ArrowRightFromLine,
  Clock,
  Cpu,
  ExternalLink,
  FlaskConical,
  Globe,
  Key,
  LineChart,
  MoreHorizontal,
  Rocket,
  ScrollText,
  Workflow,
  PlayCircle,
  PauseCircle,
  Info,
  SquareStack,
  MemoryStick,
  SlidersVertical,
} from "@wso2/oxygen-ui-icons-react";
import { generatePath, Link, useParams, useSearchParams } from "react-router-dom";
import {
  alpha,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Collapse,
  Divider,
  IconButton,
  Menu,
  MenuItem,
  Skeleton,
  Stack,
  Tooltip,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  DeploymentStatus,
  EnvStatus,
  IsolationTierBadge,
  ResourceMetricChip,
  formatUsagePercent,
  getUsagePercentVariant,
} from "@agent-management-platform/shared-component";
import { EditDeployConfigDrawer } from "./EditDeployConfigDrawer";
import {
  absoluteRouteMap,
  AgentResourceConfigsResponse,
  MetricsResponse,
  Environment,
  AgentKindVersionResponse,
  TraceListTimeRange,
} from "@agent-management-platform/types";
import { extractBuildIdFromImageId } from "../utils/extractBuildIdFromImageId";
import { normalizePythonMinor } from "../utils/instrumentation";
import { formatDistanceToNow } from "date-fns";
import { useCallback, useMemo, useState } from "react";
import { EditResourceConfigsDrawer } from "./EditResourceConfigsDrawer";
import { PromoteAgentDrawer } from "./PromoteAgentDrawer";

function DeploymentStatusPanel({ status }: { status: DeploymentStatus }) {
  const theme = useTheme();
  const backgroundColor = useMemo(() => {
    if (status === DeploymentStatus.ACTIVE) {
      return alpha(theme.palette.success.light, 0.1);
    }
    if (status === DeploymentStatus.INACTIVE) {
      return theme.vars?.palette?.Skeleton.bg;
    }
    if (status === DeploymentStatus.DEPLOYING) {
      return alpha(theme.palette.warning.light, 0.1);
    }
    if (status === DeploymentStatus.ERROR) {
      return alpha(theme.palette.error.light, 0.1);
    }
    if (status === DeploymentStatus.SUSPENDED) {
      return theme.vars?.palette?.Skeleton?.bg;
    }
    if (status === DeploymentStatus.FAILED) {
      return alpha(theme.palette.error.light, 0.1);
    }
    return theme.vars?.palette?.Skeleton?.bg;
  }, [status, theme]);

  return (
    <Box
      display="flex"
      gap={1}
      flexGrow={1}
      alignItems="center"
      justifyContent="space-between"
      sx={{
        backgroundColor: backgroundColor,
        padding: 1,
        borderRadius: 0.5,
      }}
    >
      <Typography variant="body2">Deployment Status:</Typography>
      <EnvStatus status={status} />
    </Box>
  );
}

function ResourceConfigsPanel({
  resourceConfigs,
  isLoading,
  metrics,
}: {
  resourceConfigs?: AgentResourceConfigsResponse;
  isLoading: boolean;
  metrics?: MetricsResponse;
}) {
  const lastCpu = metrics?.cpuUsage?.length
    ? metrics.cpuUsage[metrics.cpuUsage.length - 1]?.value
    : undefined;
  const lastMemory = metrics?.memory?.length
    ? metrics.memory[metrics.memory.length - 1]?.value
    : undefined;
  const lastCpuRequest = metrics?.cpuRequests?.length
    ? metrics.cpuRequests[metrics.cpuRequests.length - 1]?.value
    : undefined;
  const lastMemoryRequest = metrics?.memoryRequests?.length
    ? metrics.memoryRequests[metrics.memoryRequests.length - 1]?.value
    : undefined;
  const cpuRequest = resourceConfigs?.resources?.requests?.cpu ?? "—";
  const memoryRequest = resourceConfigs?.resources?.requests?.memory ?? "—";
  const cpuPercent =
    lastCpu !== undefined && lastCpuRequest !== undefined && lastCpuRequest > 0
      ? formatUsagePercent(lastCpu, lastCpuRequest)
      : undefined;
  const memoryPercent =
    lastMemory !== undefined &&
      lastMemoryRequest !== undefined &&
      lastMemoryRequest > 0
      ? formatUsagePercent(lastMemory, lastMemoryRequest)
      : undefined;
  const cpuVariant =
    lastCpu !== undefined && lastCpuRequest !== undefined && lastCpuRequest > 0
      ? getUsagePercentVariant(lastCpu, lastCpuRequest)
      : undefined;
  const memoryVariant =
    lastMemory !== undefined &&
      lastMemoryRequest !== undefined &&
      lastMemoryRequest > 0
      ? getUsagePercentVariant(lastMemory, lastMemoryRequest)
      : undefined;

  if (isLoading) {
    return (
      <Stack direction="row" gap={1} justifyContent="center" alignItems="center" width="100%">
        <Skeleton variant="rounded" width={"100%"} height={32} />
      </Stack>
    );
  }
  if (!resourceConfigs) {
    return (
      <NoDataFound
        message="No Resource Configs found"
        icon={<Info size={16} />}
        disableBackground
      />
    );
  }
  return (
    <Stack direction="row" spacing={1} width="100%">
      <ResourceMetricChip
        icon={<SquareStack size={16} />}
        label="Replicas"
        primaryValue={""}
        secondaryValue={
          resourceConfigs.autoScaling?.enabled
            ? "AUTO"
            : (resourceConfigs.replicas ?? "--")
        }
        secondaryTooltip={
          resourceConfigs.autoScaling?.enabled
            ? `Autoscaling is enabled, replicas can be ${resourceConfigs.autoScaling?.minReplicas} to ${resourceConfigs.autoScaling?.maxReplicas}`
            : "Autoscaling is disabled, replicas are fixed"
        }
        secondaryVariant={"success"}
      />
      <ResourceMetricChip
        icon={<Cpu size={16} />}
        label="CPU"
        primaryValue={cpuRequest}
        secondaryValue={cpuPercent}
        secondaryTooltip={
          cpuPercent ? "Current usage as % of requested." : undefined
        }
        secondaryVariant={cpuVariant}
      />
      <ResourceMetricChip
        icon={<MemoryStick size={16} />}
        label="Memory"
        primaryValue={memoryRequest}
        secondaryValue={memoryPercent}
        secondaryTooltip={
          memoryPercent ? "Current usage as % of requested." : undefined
        }
        secondaryVariant={memoryVariant}
      />
    </Stack>
  );
}
interface DeployCardProps {
  currentEnvironment: Environment;
}

const ENV_ID_PARAM = "envId";
const OPEN_RES_CONFIG_PARAM = "openResConfig";
const OPEN_PROMOTE_PARAM = "openPromote";
const OPEN_CONFIGURE_PARAM = "openConfigure";

export function DeployCard(props: DeployCardProps) {
  const { currentEnvironment } = props;
  const { orgId, agentId, projectId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const resourceConfigDrawerOpen =
    searchParams.get(OPEN_RES_CONFIG_PARAM) === "open" &&
    searchParams.get(ENV_ID_PARAM) === currentEnvironment.name;

  const promoteDrawerOpen =
    searchParams.get(OPEN_PROMOTE_PARAM) === "open" &&
    searchParams.get(ENV_ID_PARAM) === currentEnvironment.name;

  const handleOpenResourceConfigDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set(ENV_ID_PARAM, currentEnvironment.name);
    next.set(OPEN_RES_CONFIG_PARAM, "open");
    setSearchParams(next);
  }, [currentEnvironment.name, searchParams, setSearchParams]);

  const handleCloseResourceConfigDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete(OPEN_RES_CONFIG_PARAM);
    next.delete(ENV_ID_PARAM);
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const handleOpenPromoteDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set(ENV_ID_PARAM, currentEnvironment.name);
    next.set(OPEN_PROMOTE_PARAM, "open");
    setSearchParams(next);
  }, [currentEnvironment.name, searchParams, setSearchParams]);

  const handleClosePromoteDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete(OPEN_PROMOTE_PARAM);
    next.delete(ENV_ID_PARAM);
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const configureDrawerOpen =
    searchParams.get(OPEN_CONFIGURE_PARAM) === "open" &&
    searchParams.get(ENV_ID_PARAM) === currentEnvironment.name;

  const handleOpenConfigureDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.set(ENV_ID_PARAM, currentEnvironment.name);
    next.set(OPEN_CONFIGURE_PARAM, "open");
    setSearchParams(next);
  }, [currentEnvironment.name, searchParams, setSearchParams]);

  const handleCloseConfigureDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete(OPEN_CONFIGURE_PARAM);
    next.delete(ENV_ID_PARAM);
    setSearchParams(next);
  }, [searchParams, setSearchParams]);


  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments({
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    });

  const { data: pipeline } = useGetDeploymentPipeline({
    orgName: orgId,
    projName: projectId,
  });

  const hasPromotionTarget = useMemo(() => {
    if (!pipeline) return false;
    // Only show Promote when this environment has at least one downstream
    // target. The last environment in a pipeline has no outgoing promotion
    // path (or an empty target list), so the button is hidden for it.
    return pipeline.promotionPaths.some(
      (p) =>
        p.sourceEnvironmentRef === currentEnvironment.name &&
        (p.targetEnvironmentRefs?.length ?? 0) > 0,
    );
  }, [pipeline, currentEnvironment.name]);
  const { mutate: updateDeploymentState, isPending: isUpdating } =
    useUpdateDeploymentState();

  const { data: resourceConfigs, isLoading: isResourceConfigsLoading } =
    useGetAgentResourceConfigs(
      {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      },
      {
        environment: currentEnvironment.name,
      },
    );

  const currentDeployment = deployments?.[currentEnvironment.name];
  const isEnvironmentActive =
    currentDeployment?.status === DeploymentStatus.ACTIVE;

  const { data: metrics } = useGetAgentMetrics(
    {
      orgName: orgId,
      projName: projectId,
      agentName: agentId,
    },
    {
      environmentName: currentEnvironment.name,
    },
    {
      enabled:
        !!orgId &&
        !!projectId &&
        !!agentId &&
        !!currentEnvironment.name &&
        isEnvironmentActive,
      enableAutoRefresh: true,
      timeRange: TraceListTimeRange.TEN_MINUTES,
    },
  );
  const { data: agent } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const isApiAgent = agent?.agentType?.type === "agent-api";
  const isPythonBuildpack =
    agent?.build?.type === "buildpack" &&
    "buildpack" in (agent.build ?? {}) &&
    (agent.build as { buildpack?: { language?: string } }).buildpack?.language === "python";
  const agentPythonVersion = normalizePythonMinor(
    (agent?.build as { buildpack?: { languageVersion?: string } } | undefined)
      ?.buildpack?.languageVersion,
  );

  // Endpoint authentication (none/api-key/oauth) is managed in the security drawer since it
  // spans three mutually-exclusive modes plus OAuth config fields. Auto-instrumentation, the
  // instrumentation version, and tracing-token regeneration are managed in the
  // "Configurations and Secrets" drawer (EditDeployConfigDrawer).
  const [actionsMenuAnchor, setActionsMenuAnchor] = useState<null | HTMLElement>(null);

  // Per-environment config drives the overview chips. GetAgent returns only the lowest
  // environment's values, so every env's card would otherwise show the same CORS/Auth/Tracing.
  const { data: envConfig } = useGetAgentConfigurations(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { environment: currentEnvironment.name },
  );

  const authMode: "none" | "apikey" | "oauth" = envConfig?.enableOAuthSecurity
    ? "oauth"
    : envConfig?.enableApiKeySecurity
      ? "apikey"
      : "none";
  const authLabel =
    authMode === "oauth"
      ? "OAuth"
      : authMode === "apikey"
        ? "API key"
        : "None";

  const corsEnabled = envConfig?.corsConfig?.enabled ?? false;
  const corsOrigins = envConfig?.corsConfig?.allowOrigin ?? [];
  const corsDetail = corsEnabled
    ? corsOrigins.includes("*") ? "All origins" : `${corsOrigins.length} origin${corsOrigins.length !== 1 ? "s" : ""}`
    : "Disabled";

  const resilienceTimeoutSeconds = envConfig?.resilienceTimeoutSeconds ?? 30;
  const isWholeMinutes = resilienceTimeoutSeconds >= 60 && resilienceTimeoutSeconds % 60 === 0;
  const resilienceTimeoutLabel = isWholeMinutes
    ? `${resilienceTimeoutSeconds / 60}m`
    : `${resilienceTimeoutSeconds}s`;

  const kindName = agent?.kindName;

  const { data: kindVersions } = useListAgentKindVersions(
    { orgName: orgId ?? "", kindName: kindName ?? "" },
  );

  const matchedKindVersion: AgentKindVersionResponse | undefined = kindVersions?.find(
    (v) => v.imageId === currentDeployment?.imageId,
  );

  const selectedBuildId = extractBuildIdFromImageId(currentDeployment?.imageId);
  const lastDeployedText = currentDeployment?.lastDeployed
    ? formatDistanceToNow(new Date(currentDeployment.lastDeployed), {
      addSuffix: true,
    })
    : "Unknown";

  const handleStop = () => {
    if (!currentEnvironment?.name || !orgId || !projectId || !agentId) return;
    updateDeploymentState({
      params: {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      },
      body: {
        environment: currentEnvironment.name,
        state: "Undeploy",
      },
    });
  };

  const handleRedeploy = () => {
    if (!currentEnvironment?.name || !orgId || !projectId || !agentId) return;
    updateDeploymentState({
      params: {
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      },
      body: {
        environment: currentEnvironment.name,
        state: "Active",
      },
    });
  };

  if (isDeploymentsLoading) {
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

  if (!currentDeployment || currentDeployment.status === "not-deployed") {
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
              message="No Deployment found"
              subtitle={`Build your agent first to deploy it to ${currentEnvironment.displayName} environment.`}
              icon={<Rocket size={32} />}
              disableBackground
            />
          </Stack>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card
      variant="outlined"
      sx={{
        height: "fit-content",
        width: 400,
        minWidth: 400,
      }}
    >
      <CardContent>
        <Stack gap={2}>
          <Stack direction="row" gap={1} alignItems="center">
            <IsolationTierBadge tier={currentEnvironment?.isolationTier} />
            <Typography variant="h5">
              {currentEnvironment?.displayName} Environment
            </Typography>
          </Stack>
          <Divider />
          <Stack direction="row" gap={1} alignItems="center">
            <Typography variant="body2">Last Deployed</Typography>
            <Clock size={16} />
            <Typography variant="body2">{lastDeployedText}</Typography>
          </Stack>
          <Stack direction="row" gap={1} alignItems="center">
            <DeploymentStatusPanel
              status={currentDeployment?.status as DeploymentStatus}
            />
          </Stack>
          {currentDeployment?.imageId && (
            kindName ? (
              <TextInput
                label="Kind Version"
                labelAction={
                  <IconButton
                    component={Link}
                    to={
                      generatePath(
                        absoluteRouteMap.children.org.children.catalog.children.kindDetails.path,
                        { orgId, kindId: kindName },
                      ) +
                      (matchedKindVersion ? `?version=${matchedKindVersion.version}` : "")
                    }
                  >
                    <ExternalLink size={16} />
                  </IconButton>
                }
                value={matchedKindVersion ? `v${matchedKindVersion.version}` : ""}
                slotProps={{ input: { readOnly: true } }}
              />
            ) : (
              <TextInput
                label="Build Image"
                labelAction={
                  <IconButton
                    component={Link}
                    to={
                      generatePath(
                        absoluteRouteMap.children.org.children.projects.children
                          .agents.children.build.path,
                        { orgId, projectId, agentId },
                      ) +
                      "?panel=logs&selectedBuild=" +
                      selectedBuildId
                    }
                  >
                    <ExternalLink size={16} />
                  </IconButton>
                }
                value={currentDeployment?.imageId}
                copyable
                copyTooltipText="Copy Build Image"
                slotProps={{ input: { readOnly: true } }}
              />
            )
          )}
          {currentDeployment?.endpoints.map((endpoint) => (
            <TextInput
              key={endpoint.url}
              label="URL"
              value={endpoint.url}
              copyable
              copyTooltipText="Copy URL"
              slotProps={{
                input: {
                  readOnly: true,
                },
              }}
            />
          ))}

          <Collapse in={[
            DeploymentStatus.ACTIVE, DeploymentStatus.ERROR, DeploymentStatus.FAILED,
          ].includes(currentDeployment?.status as DeploymentStatus)}>
            <Stack gap={2}>
              <Card variant="outlined" sx={{ padding: 1.4, pt: 0.5 }}>
                <Stack gap={1}>
                  <Stack direction="row" gap={1} alignItems="center" justifyContent="space-between">
                    <Typography variant="h6">Resource Usage</Typography>
                    <Button
                      variant="text"
                      size="small"
                      color="inherit"
                      sx={{ padding: 0.5 }}
                      startIcon={<SlidersVertical size={16} />}
                      onClick={handleOpenResourceConfigDrawer}
                    >
                      Configure
                    </Button>
                  </Stack>
                  <Stack direction="row" gap={1} alignItems="center">
                    <ResourceConfigsPanel
                      resourceConfigs={resourceConfigs}
                      isLoading={isResourceConfigsLoading}
                      metrics={metrics}
                    />
                  </Stack>
                </Stack>
              </Card>

              <Card variant="outlined" sx={{ padding: 1.4 }}>
                <Stack gap={1.5}>
                  {/* One Configure opens the unified drawer (CORS, Authentication, Tracing,
                      Environment Variables, File Mounts). Rows below are read-only overviews. */}
                  <Stack direction="row" gap={1} alignItems="center" justifyContent="space-between">
                    <Typography variant="h6">Configurations and Secrets</Typography>
                    <Button
                      variant="text"
                      size="small"
                      color="inherit"
                      sx={{ padding: 0.5 }}
                      startIcon={<SlidersVertical size={16} />}
                      onClick={handleOpenConfigureDrawer}
                      disabled={currentDeployment?.status === DeploymentStatus.DEPLOYING}
                    >
                      Configure
                    </Button>
                  </Stack>

                  {/* CORS overview */}
                  {isApiAgent && (
                    <Box display="flex" alignItems="center" gap={1}>
                      <Globe size={14} style={{ opacity: 0.6 }} />
                      <Typography variant="body2">CORS</Typography>
                      <Tooltip title={corsDetail}>
                        <Chip
                          size="small"
                          label={corsEnabled ? "On" : "Off"}
                          color={corsEnabled ? "success" : "default"}
                          variant="outlined"
                          sx={{ height: 18, fontSize: "0.65rem", cursor: "default" }}
                        />
                      </Tooltip>
                    </Box>
                  )}

                  {/* Endpoint Authentication overview */}
                  {isApiAgent && (
                    <Box display="flex" alignItems="center" gap={1}>
                      <Key size={14} style={{ opacity: 0.6 }} />
                      <Typography variant="body2">Authentication</Typography>
                      <Tooltip
                        title={
                          authMode === "oauth"
                            ? "Callers send an Authorization: Bearer <token> header validated by the gateway"
                            : authMode === "apikey"
                              ? "Requests must include the header: x-api-key: <your-key>"
                              : "Endpoint is publicly accessible without authentication"
                        }
                      >
                        <Chip
                          size="small"
                          label={authLabel}
                          color={authMode === "none" ? "default" : "success"}
                          variant="outlined"
                          sx={{ height: 18, fontSize: "0.65rem", cursor: "default" }}
                        />
                      </Tooltip>
                    </Box>
                  )}
                  {isApiAgent && (
                    <Box display="flex" alignItems="center" gap={1}>
                      <Clock size={14} style={{ opacity: 0.6 }} />
                      <Typography variant="body2">Gateway Timeout</Typography>
                      <Tooltip title="Max duration the gateway keeps a response open between this agent and the client before cutting it off">
                        <Chip
                          size="small"
                          label={resilienceTimeoutLabel}
                          color="default"
                          variant="outlined"
                          sx={{ height: 18, fontSize: "0.65rem", cursor: "default" }}
                        />
                      </Tooltip>
                    </Box>
                  )}

                  {/* Tracing - Instrumentation overview */}
                  {isPythonBuildpack && (
                    <Box display="flex" alignItems="center" gap={1}>
                      <Workflow size={14} style={{ opacity: 0.6 }} />
                      <Typography variant="body2">Tracing - Instrumentation</Typography>
                      <Chip
                        size="small"
                        label={envConfig?.enableAutoInstrumentation ? "On" : "Off"}
                        color={envConfig?.enableAutoInstrumentation ? "success" : "default"}
                        variant="outlined"
                        sx={{ height: 18, fontSize: "0.65rem", cursor: "default" }}
                      />
                    </Box>
                  )}

                  {/* Environment variables & file mounts (secrets) overview */}
                  <Box display="flex" alignItems="center" gap={1}>
                    <SlidersVertical size={14} style={{ opacity: 0.6 }} />
                    <Typography variant="body2">Environment & Secrets</Typography>
                  </Box>
                </Stack>
              </Card>
            </Stack>
          </Collapse>
          {agentId && (
            <EditResourceConfigsDrawer
              open={resourceConfigDrawerOpen}
              onClose={handleCloseResourceConfigDrawer}
              resourceConfigs={resourceConfigs}
              orgName={orgId ?? "default"}
              projName={projectId ?? "default"}
              agentName={agentId}
              environment={currentEnvironment.name}
            />
          )}
          {agentId && (
            <EditDeployConfigDrawer
              open={configureDrawerOpen}
              onClose={handleCloseConfigureDrawer}
              mode="update"
              orgName={orgId ?? ""}
              projName={projectId ?? ""}
              agentName={agentId}
              environment={currentEnvironment.name}
              title="Configurations and Secrets"
              isApiAgent={isApiAgent}
              isPythonBuildpack={isPythonBuildpack}
              agentPythonVersion={agentPythonVersion}
            />
          )}
          {agentId && orgId && projectId && (
            <PromoteAgentDrawer
              open={promoteDrawerOpen}
              onClose={handleClosePromoteDrawer}
              sourceEnvironment={currentEnvironment}
              orgId={orgId}
              projectId={projectId}
              agentId={agentId}
            />
          )}
          {agent?.provisioning?.type === "internal" && (
            <>
              <Divider />
              <Stack direction="row" justifyContent="space-between" spacing={1} alignItems="center">
                <Tooltip title="More actions">
                  <IconButton size="small" onClick={(e) => setActionsMenuAnchor(e.currentTarget)}>
                    <MoreHorizontal size={18} />
                  </IconButton>
                </Tooltip>
                <Stack direction="row" justifyContent="right" spacing={1} alignItems="center">
                {currentDeployment?.status !== DeploymentStatus.SUSPENDED && (
                  <Button
                    startIcon={<PauseCircle size={16} />}
                    variant="text"
                    size="small"
                    onClick={handleStop}
                    disabled={
                      isUpdating ||
                      currentDeployment?.status !== DeploymentStatus.ACTIVE
                    }
                  >
                    Suspend
                  </Button>
                )}
                {currentDeployment?.status === DeploymentStatus.SUSPENDED && (
                  <Button
                    startIcon={
                      isUpdating ? (
                        <CircularProgress size={14} />
                      ) : (
                        <PlayCircle size={16} />
                      )
                    }
                    variant="text"
                    color="success"
                    size="small"
                    onClick={handleRedeploy}
                    disabled={isUpdating}
                  >
                    Re-deploy
                  </Button>
                )}
                {hasPromotionTarget && (
                  <>
                    <Divider orientation="vertical" flexItem />
                    <Button
                      variant="contained"
                      size="small"
                      startIcon={<ArrowRightFromLine size={16} />}
                      onClick={handleOpenPromoteDrawer}
                      disabled={!isEnvironmentActive}
                    >
                      Promote
                    </Button>
                  </>
                )}
                </Stack>
                <Menu
                  anchorEl={actionsMenuAnchor}
                  open={Boolean(actionsMenuAnchor)}
                  onClose={() => setActionsMenuAnchor(null)}
                  anchorOrigin={{ vertical: "top", horizontal: "right" }}
                  transformOrigin={{ vertical: "bottom", horizontal: "right" }}
                >
                  <MenuItem
                    component={Link}
                    to={generatePath(
                      absoluteRouteMap.children.org.children.projects.children.agents
                        .children.environment.children.tryOut.path,
                      { orgId, projectId, agentId, envId: currentEnvironment?.name },
                    )}
                    onClick={() => setActionsMenuAnchor(null)}
                  >
                    <FlaskConical size={16} style={{ marginRight: 8 }} />
                    Test Agent
                  </MenuItem>
                  <MenuItem
                    component={Link}
                    to={generatePath(
                      absoluteRouteMap.children.org.children.projects.children.agents
                        .children.environment.children.observability.children.traces.path,
                      { orgId, projectId, agentId, envId: currentEnvironment?.name },
                    )}
                    onClick={() => setActionsMenuAnchor(null)}
                  >
                    <Workflow size={16} style={{ marginRight: 8 }} />
                    View Traces
                  </MenuItem>
                  <MenuItem
                    component={Link}
                    to={generatePath(
                      absoluteRouteMap.children.org.children.projects.children.agents
                        .children.environment.children.observability.children.logs.path,
                      { orgId, projectId, agentId, envId: currentEnvironment?.name },
                    )}
                    onClick={() => setActionsMenuAnchor(null)}
                  >
                    <ScrollText size={16} style={{ marginRight: 8 }} />
                    View Logs
                  </MenuItem>
                  <MenuItem
                    component={Link}
                    to={generatePath(
                      absoluteRouteMap.children.org.children.projects.children.agents
                        .children.environment.children.observability.children.metrics.path,
                      { orgId, projectId, agentId, envId: currentEnvironment?.name },
                    )}
                    onClick={() => setActionsMenuAnchor(null)}
                  >
                    <LineChart size={16} style={{ marginRight: 8 }} />
                    View Metrics
                  </MenuItem>
                  <MenuItem
                    component={Link}
                    to={generatePath(
                      absoluteRouteMap.children.org.children.projects.children.agents
                        .children.environment.children.security.path,
                      { orgId, projectId, agentId, envId: currentEnvironment?.name },
                    )}
                    onClick={() => setActionsMenuAnchor(null)}
                  >
                    <Key size={16} style={{ marginRight: 8 }} />
                    Manage Credentials
                  </MenuItem>
                </Menu>
              </Stack>
            </>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}

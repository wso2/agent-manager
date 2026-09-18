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
  useDeployedAgentKindVersion,
  useGetAgent,
  useGetAllAgentBuilds,
  useListAgentDeployments,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  Environment,
} from "@agent-management-platform/types";
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Skeleton,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  CheckCircle as CheckCircleRounded,
  Circle as CircleOutlined,
  Rocket as RocketLaunchOutlined,
  PauseCircle,
  Play,
} from "@wso2/oxygen-ui-icons-react";
import { NoDataFound } from "@agent-management-platform/views";
import { generatePath, Link } from "react-router-dom";

/**
 * The agent's Deploy page — shared by every "Go to Deployment" / "Promote" /
 * "View Deployment" link on this card.
 */
export function getAgentDeploymentPath(orgId: string, projectId: string, agentId: string): string {
  return generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents.children.deployment.path,
    { orgId, projectId, agentId },
  );
}

/** The agent's per-environment Security (API key) page. */
export function getAgentSecurityPath(
  orgId: string, projectId: string, agentId: string, envId: string,
): string {
  return generatePath(
    absoluteRouteMap.children.org.children.projects.children.agents
      .children.environment.children.security.path,
    { orgId, projectId, agentId, envId },
  );
}

export enum DeploymentStatus {
  ACTIVE = "active",
  INACTIVE = "not-deployed",
  DEPLOYING = "in-progress",
  ERROR = "error",
  SUSPENDED = "suspended",
  FAILED = "failed",
}

export interface EnvironmentCardProps {
  environment?: Environment;
  orgId: string;
  projectId: string;
  agentId: string;
  actions?: React.ReactNode;
  /**
   * Rendered below the header/tabs row, in every render branch
   * (external, not-yet-deployed, deployed) regardless of deployment status.
   * This card no longer lists `currentDeployment.endpoints` itself (see
   * EnvironmentCard.tsx history) — a caller that wants endpoint/invoke-URL
   * visibility must render it here, as pages/overview's
   * EnvCapabilitiesSection does. Individual sections (Capabilities, Configs,
   * Agent Identity, Deployment Status, Monitors, Traces) each decide for
   * themselves whether they have anything to show; Monitors/Traces already
   * hide themselves while there's no live deployment traffic to report on.
   */
  bottomContent?: React.ReactNode;
  /**
   * Whether this is the first (root) environment of the deployment pipeline.
   * The root env is reached by deploying a build directly; downstream envs are
   * reached by promoting from the previous environment. Defaults to true so
   * callers without pipeline context keep the deploy-oriented wording.
   */
  isFirstEnvironment?: boolean;
  /** Replaces the environment name heading, e.g. a tab strip switching between sibling envs. */
  tabsHeader?: React.ReactNode;
  /**
   * Suppresses the environment name heading entirely (no tabsHeader, no
   * fallback title) — e.g. when there's only one environment and naming it
   * adds no information.
   */
  hideEnvTitle?: boolean;
}

export const EnvStatus = ({
  status,
  suffix,
}: {
  status?: DeploymentStatus;
  suffix?: string;
}) => {
  const theme = useTheme();
  if (!status) {
    return null;
  }
  if (status === DeploymentStatus.ACTIVE) {
    return (
      <Chip
        icon={
          <CheckCircleRounded size={16} color={theme.vars?.palette?.success?.main} />
        }
        variant="outlined"
        size="small"
        label={suffix ? `Deployed · ${suffix}` : "Deployed"}
        color="success"
      />
    );
  }
  if (status === DeploymentStatus.INACTIVE) {
    return (
      <Chip
        icon={<CircleOutlined size={16} color={theme.vars?.palette?.text?.disabled} />}
        variant="outlined"
        size="small"
        label="Not Deployed"
        color="default"
      />
    );
  }
  if (status === DeploymentStatus.DEPLOYING) {
    return (
      <Chip
        icon={<CircularProgress size={16} color="warning" />}
        variant="outlined"
        size="small"
        label="Deploying"
        color="warning"
      />
    );
  }
  if (status === DeploymentStatus.ERROR) {
    return <Chip variant="outlined" size="small" label="Error" color="error" />;
  }
  if (status === DeploymentStatus.FAILED) {
    return <Chip variant="outlined" size="small" label="Error" color="error" />;
  }
  if (status === DeploymentStatus.SUSPENDED) {
    return (
      <Chip
        icon={<PauseCircle size={16} />}
        variant="outlined"
        size="small"
        label="Suspended"
        color="default"
      />
    );
  }
};

export const EnvironmentCard = (props: EnvironmentCardProps) => {
  const {
    environment,
    orgId,
    projectId,
    agentId,
    actions,
    bottomContent,
    isFirstEnvironment = true,
    tabsHeader,
    hideEnvTitle,
  } = props;
  const { data: agent, isLoading: isAgentLoading } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });

  const isExternal = agent?.provisioning?.type === "external";

  const { data: deployments, isLoading: isDeploymentsLoading } =
    useListAgentDeployments(
      { orgName: orgId, projName: projectId, agentName: agentId },
      { enabled: !!orgId && !!projectId && !!agentId && !!agent && !isExternal }
    );

  const kindName = agent?.kindName;
  const currentDeployment = deployments?.[environment?.name ?? ""];
  const envTitle = environment?.displayName ?? environment?.name ?? "Environment";

  const { data: buildsData } = useGetAllAgentBuilds({
    orgName: !isExternal ? orgId : "",
    projName: !isExternal ? projectId : "",
    agentName: !isExternal ? agentId : "",
  });

  const hasSuccessfulBuild = buildsData?.builds?.some(
    (b) => b.status === "Succeeded" || b.status === "Completed"
  ) ?? false;

  const { deployedVersion, latestKindVersion } = useDeployedAgentKindVersion({
    orgName: orgId,
    kindName,
    imageId: currentDeployment?.imageId,
  });

  const isKindOutdated =
    !!kindName &&
    !!latestKindVersion &&
    !!deployedVersion &&
    deployedVersion !== latestKindVersion.version;

  if (isAgentLoading || isDeploymentsLoading) {
    return <Skeleton variant="rounded" height={100} />;
  }

  // ── External agent ────────────────────────────────────────────────────────
  if (isExternal) {
    return (
      <Card variant="outlined">
        <CardContent>
          <Box display="flex" flexDirection="row" gap={1} justifyContent="space-between" alignItems="center">
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              {!hideEnvTitle && (tabsHeader ?? <Typography variant="h6">{envTitle}</Typography>)}
            </Box>
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              {actions}
            </Box>
          </Box>
          {bottomContent}
        </CardContent>
      </Card>
    );
  }

  // ── Internal agent — not yet deployed ─────────────────────────────────────
  if (!currentDeployment) {
    return (
      <Card variant="outlined" sx={{ "&.MuiCard-root": { backgroundColor: "background.paper" } }}>
        <CardContent>
          <Box display="flex" flexDirection="row" gap={1} justifyContent="space-between" alignItems="center">
            <Box display="flex" flexDirection="row" gap={1} alignItems="center">
              {!hideEnvTitle && (tabsHeader ?? <Typography variant="h6">{envTitle}</Typography>)}
            </Box>
          </Box>
          {bottomContent}
        </CardContent>
      </Card>
    );
  }

  // ── Internal agent — deployment exists ────────────────────────────────────
  // The status-message block below bottomContent — computed once (rather
  // than a separate `hasStatusMessage` boolean re-deriving the same status
  // union) so the "is there anything to show" check and the render can never
  // drift apart. A plain active, up-to-date deployment resolves to null, so
  // the divider before it is skipped too.
  const statusMessage =
    currentDeployment.status === DeploymentStatus.INACTIVE ? (
      <NoDataFound
        disableBackground
        message="Not Deployed"
        icon={<RocketLaunchOutlined size={32} />}
        subtitle={
          hasSuccessfulBuild
            ? isFirstEnvironment
              ? "A successful build is available. Deploy it to get started."
              : "Promote a deployment from the previous environment to get started."
            : "No successful build found. Build the agent before deploying."
        }
        action={
          hasSuccessfulBuild && (
            <Button
              startIcon={<RocketLaunchOutlined size={16} />}
              variant="outlined"
              component={Link}
              to={getAgentDeploymentPath(orgId, projectId, agentId)}
              size="small"
            >
              {isFirstEnvironment ? "Go to Deployment" : "Promote"}
            </Button>
          )
        }
      />
    ) : currentDeployment.status === DeploymentStatus.DEPLOYING ? (
      <NoDataFound disableBackground message="Deploying..." icon={<CircularProgress size={32} />} />
    ) : currentDeployment.status === DeploymentStatus.ERROR ||
      currentDeployment.status === DeploymentStatus.FAILED ? (
      <Alert
        severity="error"
        sx={{ width: "100%" }}
        action={
          <Button
            component={Link}
            to={getAgentDeploymentPath(orgId, projectId, agentId)}
            color="inherit"
            size="small"
          >
            View Deployment
          </Button>
        }
      >
        Deployment failed. Check the deployment page for more details.
      </Alert>
    ) : currentDeployment.status === DeploymentStatus.SUSPENDED ? (
      <NoDataFound
        disableBackground
        message="Suspended"
        icon={<PauseCircle size={32} />}
        subtitle="This deployment is currently suspended. Resume it from the deployment page to make the agent available again."
        action={
          <Button
            startIcon={<Play size={16} />}
            variant="outlined"
            component={Link}
            to={getAgentDeploymentPath(orgId, projectId, agentId)}
            size="small"
          >
            Go to Deployment
          </Button>
        }
      />
    ) : currentDeployment.status === DeploymentStatus.ACTIVE && isKindOutdated ? (
      <Alert severity="warning" sx={{ width: "100%" }}>
        A newer version of this Agent Kind is available: <strong>v{latestKindVersion!.version}</strong>.{" "}
        Currently deployed: <strong>v{deployedVersion}</strong>.
      </Alert>
    ) : null;
  return (
    <Card variant="outlined">
      <CardContent>
        <Box
          display="flex"
          flexDirection="row"
          gap={1}
          justifyContent="space-between"
          alignItems="center"
        >
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            {!hideEnvTitle && (
              tabsHeader ?? (
                <Typography variant="h6">
                  {envTitle}
                </Typography>
              )
            )}
          </Box>
          <Box display="flex" flexDirection="row" gap={1} alignItems="center">
            {currentDeployment?.status === DeploymentStatus.ACTIVE && actions}
          </Box>
        </Box>
        {bottomContent}
        {statusMessage && (
          <>
            <Divider />
            <Box
              display="flex"
              width="100%"
              justifyContent="center"
              flexDirection="column"
              gap={1}
              pt={2}
              alignItems="center"
            >
              {statusMessage}
            </Box>
          </>
        )}
      </CardContent>
    </Card>
  );
};

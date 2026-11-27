/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import { useState } from "react";
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  ListItemText,
  Menu,
  MenuItem,
  Stack,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, UserPlus } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useLocation, useNavigate } from "react-router-dom";
import { TextInput, returnToState } from "@agent-management-platform/views";
import { useListProjects, useShadowConnection } from "@agent-management-platform/api-client";
import { useFeatureAccess } from "@agent-management-platform/feature-management";
import {
  absoluteRouteMap,
  type AgentSourceSeedState,
  type ShadowFindingDetail,
} from "@agent-management-platform/types";
import { buildAgentSourceSeed, platformHostedBlocker } from "./platformHostedSeed";

/** Asks which project to deploy the code agent to, then opens the platform-hosted agent form. */
function DeployDialog({
  open,
  orgName,
  onClose,
  onContinue,
}: {
  open: boolean;
  orgName: string;
  onClose: () => void;
  onContinue: (projectName: string) => void;
}) {
  const projects = useListProjects({ orgName });
  const list = projects.data?.projects ?? [];
  const [chosen, setChosen] = useState("");
  const project = chosen || list[0]?.name || "";
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>Deploy as platform-hosted agent</DialogTitle>
      <DialogContent>
        <Stack spacing={2}>
          <Typography variant="body2" color="text.secondary">
            Agent Manager builds the agent from its repository and runs it. Choose a project, then
            check the values filled in from the code.
          </Typography>
          {projects.isLoading ? null : projects.error ? (
            <Alert severity="error">Could not load the projects.</Alert>
          ) : list.length === 0 ? (
            <Alert severity="info">There are no projects. Create a project first.</Alert>
          ) : (
            <TextInput
              select
              label="Project"
              value={project}
              onChange={(e) => setChosen(e.target.value)}
            >
              {list.map((p) => (
                <MenuItem key={p.name} value={p.name}>
                  {p.displayName || p.name}
                </MenuItem>
              ))}
            </TextInput>
          )}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ justifyContent: "flex-start", px: 3, pb: 2 }}>
        <Button variant="outlined" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="contained" disabled={!project} onClick={() => onContinue(project)}>
          Continue
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/**
 * The Onboard control of a code agent: register it as an external agent (it
 * keeps running where it is), or deploy it as a platform-hosted agent.
 */
export function CodeAgentOnboardActions({
  f,
  orgName,
  onboardPath,
}: {
  f: ShadowFindingDetail;
  orgName: string;
  onboardPath: string;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const canCreateAgent = useFeatureAccess("agent_create");
  const { data: connection, error: connectionError } = useShadowConnection(
    orgName,
    f.connectionId,
  );
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [deployOpen, setDeployOpen] = useState(false);
  const blocker = platformHostedBlocker(f, connection, canCreateAgent, !!connectionError);

  const deploy = (projectName: string) => {
    if (!connection) return;
    const state: AgentSourceSeedState = {
      ...returnToState(location),
      agentSourceSeed: buildAgentSourceSeed(f, connection, orgName),
    };
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.projects.children.newAgent.children.create.children
          .source.path,
        { orgId: orgName, projectId: projectName },
      ),
      { state },
    );
  };

  const deployItem = (
    <MenuItem
      disabled={!!blocker}
      onClick={() => {
        setAnchor(null);
        setDeployOpen(true);
      }}
    >
      <ListItemText
        primary="Deploy as platform-hosted agent"
        secondary="Agent Manager builds it from the repository and runs it."
      />
    </MenuItem>
  );

  return (
    <>
      <Button
        variant="contained"
        startIcon={<UserPlus size={16} />}
        endIcon={<ChevronDown size={16} />}
        onClick={(e) => setAnchor(e.currentTarget)}
        aria-controls={anchor ? "onboard-menu" : undefined}
        aria-haspopup="true"
        aria-expanded={Boolean(anchor)}
      >
        Manage
      </Button>
      <Menu
        id="onboard-menu"
        anchorEl={anchor}
        open={Boolean(anchor)}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
        transformOrigin={{ vertical: "top", horizontal: "right" }}
      >
        <MenuItem
          onClick={() => {
            setAnchor(null);
            navigate(onboardPath);
          }}
        >
          <ListItemText
            primary="Manage as external agent"
            secondary="It keeps running where it is. Agent Manager observes it."
          />
        </MenuItem>
        {blocker ? (
          <Tooltip title={blocker} placement="left">
            <Box component="span" sx={{ display: "block" }}>
              {deployItem}
            </Box>
          </Tooltip>
        ) : (
          deployItem
        )}
      </Menu>
      {deployOpen && (
        <DeployDialog
          open
          orgName={orgName}
          onClose={() => setDeployOpen(false)}
          onContinue={deploy}
        />
      )}
    </>
  );
}

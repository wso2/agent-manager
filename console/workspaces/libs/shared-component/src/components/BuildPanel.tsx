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
  useBuildAgent,
  useGetAgent,
  useListCommits,
} from "@agent-management-platform/api-client";
import { Wrench } from "@wso2/oxygen-ui-icons-react";
import {
  Alert,
  Box,
  Button,
  Typography,
  Select,
  MenuItem,
  SelectChangeEvent,
  CircularProgress,
  FormControl,
  FormHelperText,
  Chip,
} from "@wso2/oxygen-ui";
import { DrawerHeader, DrawerContent } from "@agent-management-platform/views";
import { useMemo, useState } from "react";
import { parseGitHubUrl } from "../utils/githubUrl";

interface BuildPanelProps {
  onClose: () => void;
  orgName: string;
  projName: string;
  agentName: string;
}

export function BuildPanel({
  onClose,
  orgName,
  projName,
  agentName,
}: BuildPanelProps) {
  const [commitId, setCommitId] = useState<string>("");

  const { mutate: buildAgent, isPending } = useBuildAgent();
  const { data: agent, isLoading: isLoadingAgent } = useGetAgent({
    orgName,
    projName,
    agentName,
  });

  // Get the branch from the agent's repository configuration
  const selectedBranch = agent?.provisioning?.repository?.branch || "";

  // Parse repository URL to get owner and repo name
  const repoInfo = useMemo(() => {
    const repoUrl = agent?.provisioning?.repository?.url;
    return repoUrl ? parseGitHubUrl(repoUrl) : null;
  }, [agent?.provisioning?.repository?.url]);

  // Get secretRef for private repository authentication
  const secretRef = agent?.provisioning?.repository?.secretRef;

  // Fetch commits for selected branch
  const {
    data: commitsData,
    isLoading: isLoadingCommits,
    isError: isCommitsError,
  } = useListCommits(
    orgName,
    {
      owner: repoInfo?.owner || "",
      repo: repoInfo?.repo || "",
      branch: selectedBranch || undefined,
      projectName: projName,
      componentName: agentName,
      // Include secretRef for private repo support; the org that scopes it comes
      // from the URL/token, not the body.
      ...(secretRef ? { secretRef: secretRef } : {}),
    },
    { limit: 50 },
    !!repoInfo && !!selectedBranch,
  );

  const commits = commitsData?.commits || [];

  // An empty commitId means "use latest"; resolve it to the first commit's
  // sha here so the build request never sends "" (which produces builds
  // with no commit hash).
  const handleCommitChange = (event: SelectChangeEvent<string>) => {
    setCommitId(event.target.value);
  };

  const handleBuild = async () => {
    try {
      const resolvedCommitId =
        !isCommitsError && !commitId ? commits[0]?.sha || "" : commitId;

      buildAgent(
        {
          params: {
            orgName,
            projName,
            agentName,
          },
          query: {
            commitId: isCommitsError ? "" : resolvedCommitId,
          },
        },
        {
          onSuccess: () => {
            onClose();
          },
        },
      );
    } catch {
      // Build trigger failed - error handling can be added here if needed
    }
  };

  return (
    <Box display="flex" flexDirection="column" height="100%">
      <DrawerHeader
        icon={<Wrench size={24} />}
        title="Trigger Build"
        onClose={onClose}
      />
      <DrawerContent>
        <Typography variant="body2" color="text.secondary">
          Build {agent?.displayName || agentName} from the latest commit, or
          choose a specific one below.
        </Typography>

        <Box display="flex" flexDirection="column" gap={2}>
          {isCommitsError ? (
            <Alert severity="warning">
              Failed to load commits. The build will use the latest commit from
              the branch.
            </Alert>
          ) : (
            <Box>
              <Typography variant="body2" sx={{ mb: 0.5, fontWeight: 500 }}>
                Commit
              </Typography>
              <FormControl fullWidth size="small">
                <Select
                  displayEmpty
                  id="commit-select"
                  value={commitId || ""}
                  onChange={handleCommitChange}
                  disabled={isLoadingCommits || !selectedBranch}
                  renderValue={(selected) => {
                    if (!selected) {
                      return (
                        <Typography variant="body2" color="text.secondary">
                          Latest commit
                        </Typography>
                      );
                    }
                    const commit = commits.find((c) => c.sha === selected);
                    if (commit) {
                      return (
                        <Box display="flex" alignItems="center" gap={1}>
                          <Typography variant="body2" noWrap>
                            {commit.message?.split("\n")[0] || commit.shortSha}
                          </Typography>
                        </Box>
                      );
                    }
                    return selected;
                  }}
                  endAdornment={
                    isLoadingCommits ? (
                      <CircularProgress size={20} sx={{ mr: 2 }} />
                    ) : undefined
                  }
                  MenuProps={{
                    PaperProps: {
                      style: {
                        maxHeight: 300,
                      },
                    },
                  }}
                >
                  {commits.map((commit, index) => (
                    <MenuItem key={commit.sha} value={commit.sha}>
                      <Box display="flex" flexDirection="column" width="100%">
                        <Box display="flex" alignItems="center" gap={1}>
                          <Typography
                            variant="body2"
                            noWrap
                            sx={{ maxWidth: 350 }}
                          >
                            {commit.message?.split("\n")[0] || ""}
                          </Typography>
                          {index === 0 && (
                            <Chip label="Latest" size="small" color="primary" variant="outlined" />
                          )}
                        </Box>
                        <Typography variant="caption" color="text.secondary">
                          {commit.shortSha}
                        </Typography>
                      </Box>
                    </MenuItem>
                  ))}
                </Select>
                <FormHelperText>
                  Select a commit to build, or leave unselected to use the
                  latest
                </FormHelperText>
              </FormControl>
            </Box>
          )}
        </Box>

        <Box display="flex" gap={1} justifyContent="flex-end" width="100%">
          <Button variant="outlined" color="primary" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="contained"
            color="primary"
            onClick={handleBuild}
            startIcon={<Wrench size={16} />}
            disabled={isPending || isLoadingAgent || !selectedBranch}
          >
            Trigger Build
          </Button>
        </Box>
      </DrawerContent>
    </Box>
  );
}

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
  Box,
  Button,
  Typography,
  Select,
  MenuItem,
  SelectChangeEvent,
  CircularProgress,
  FormControl,
  InputLabel,
  FormHelperText,
  Chip,
} from "@wso2/oxygen-ui";
import { DrawerHeader, DrawerContent } from "@agent-management-platform/views";
import { useMemo, useState } from "react";

interface BuildPanelProps {
  onClose: () => void;
  orgName: string;
  projName: string;
  agentName: string;
}

/**
 * Parses a GitHub repository URL to extract owner and repository name.
 * Supports formats:
 * - https://github.com/owner/repo
 * - https://github.com/owner/repo.git
 * - git@github.com:owner/repo.git
 */
function parseGitHubUrl(url: string): { owner: string; repo: string } | null {
  if (!url) return null;

  // Handle HTTPS URLs: https://github.com/owner/repo or https://github.com/owner/repo.git
  const httpsMatch = url.match(/github\.com\/([^/]+)\/([^/.]+)/);
  if (httpsMatch) {
    return { owner: httpsMatch[1], repo: httpsMatch[2] };
  }

  // Handle SSH URLs: git@github.com:owner/repo.git
  const sshMatch = url.match(/github\.com:([^/]+)\/([^/.]+)/);
  if (sshMatch) {
    return { owner: sshMatch[1], repo: sshMatch[2] };
  }

  return null;
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

  // Fetch commits for selected branch
  const { data: commitsData, isLoading: isLoadingCommits } = useListCommits(
    {
      owner: repoInfo?.owner || "",
      repo: repoInfo?.repo || "",
      branch: selectedBranch || undefined,
    },
    { limit: 50 },
    !!repoInfo && !!selectedBranch,
  );

  // Keep commitId empty to use latest commit (backend will determine)
  // User can explicitly select a specific commit if needed

  const handleCommitChange = (event: SelectChangeEvent<string>) => {
    setCommitId(event.target.value);
  };

  const handleBuild = async () => {
    try {
      buildAgent(
        {
          params: {
            orgName,
            projName,
            agentName,
          },
          query: {
            commitId: commitId || "",
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

  const commits = commitsData?.commits || [];

  return (
    <Box display="flex" flexDirection="column" height="100%">
      <DrawerHeader
        icon={<Wrench size={24} />}
        title="Trigger Build"
        onClose={onClose}
      />
      <DrawerContent>
        <Typography variant="body2" color="text.secondary">
          Build {agent?.displayName || agentName} from a specific commit.
        </Typography>

        <Box display="flex" flexDirection="column" gap={2}>
          <FormControl fullWidth size="small">
            <InputLabel id="commit-select-label" shrink>
              Commit
            </InputLabel>
            <Select
              notched
              displayEmpty
              labelId="commit-select-label"
              id="commit-select"
              value={commitId || ""}
              label="Commit"
              onChange={handleCommitChange}
              disabled={isLoadingCommits || !selectedBranch}
              renderValue={(selected) => {
                if (!selected) {
                  return (
                    <Typography variant="body2" color="text.secondary">
                      Using latest commit
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
              {commits.length === 0 && (
                <MenuItem value="" disabled>
                  <Typography variant="body2" color="text.secondary">
                    Using latest commit
                  </Typography>
                </MenuItem>
              )}
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
                        <Chip label="Latest" size="small" color="primary" />
                      )}
                    </Box>
                    <Typography variant="caption" color="text.secondary">
                      {commit.shortSha}
                    </Typography>
                  </Box>
                </MenuItem>
              ))}
            </Select>
            <FormHelperText>Select the commit to build</FormHelperText>
          </FormControl>
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

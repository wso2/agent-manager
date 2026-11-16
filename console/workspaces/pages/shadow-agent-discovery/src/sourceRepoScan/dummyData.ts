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

// Method 2 (source repository scan) is built separately. Until its API
// exists, this file is the only data source of the source repository
// preview on a scan's findings page (SAMPLE DATA): replace
// getSourceRepoScanData() with the real API call and keep the types.

/** A detector from the design's source repository scan (D1 to D12). */
export interface RepoDetector {
  id: string;
  name: string;
  tier: "High" | "Medium" | "Low" | "Info";
  weight: number;
  looksAt: string;
}

/** One detector hit: where it was found, never the code itself. */
export interface RepoDetectorHit {
  detectorId: string;
  path: string;
  line: number;
  symbol: string;
}

/** A suspect agent: one service folder of a repository, scored and judged. */
export interface RepoSuspect {
  id: string;
  repository: string;
  host: "GitHub" | "GitLab" | "Azure DevOps";
  servicePath: string;
  commitSha: string;
  score: number;
  verdict: "Agent" | "Possible agent" | "LLM consumer";
  hits: RepoDetectorHit[];
  /** Hard-coded secrets: type and location only, never the value. */
  secretFindings: { type: string; path: string; line: number }[];
  lastCommitAt: string;
  lastCommitter: string;
}

export interface SourceRepoScanData {
  detectors: RepoDetector[];
  connectedHosts: { host: string; organization: string; repositories: number }[];
  lastScanAt: string;
  suspects: RepoSuspect[];
}

/** The detectors and weights listed in the design document. */
export const REPO_DETECTORS: RepoDetector[] = [
  {
    id: "D1",
    name: "Agent framework dependency",
    tier: "High",
    weight: 40,
    looksAt: "Manifests and lockfiles",
  },
  { id: "D2", name: "Agent framework import", tier: "High", weight: 20, looksAt: "Source code" },
  {
    id: "D3",
    name: "MCP client config",
    tier: "High",
    weight: 35,
    looksAt: ".mcp.json, mcp.json, Spring application.yml",
  },
  { id: "D4", name: "MCP client code", tier: "High", weight: 35, looksAt: "Source code" },
  {
    id: "D5",
    name: "A2A artifact",
    tier: "High",
    weight: 35,
    looksAt: ".well-known/agent-card.json, A2A SDK",
  },
  {
    id: "D6",
    name: "Agent platform IaC",
    tier: "High",
    weight: 35,
    looksAt: "Terraform, CloudFormation, Bicep",
  },
  {
    id: "D7",
    name: "Tool calling with a model SDK",
    tier: "Medium",
    weight: 20,
    looksAt: "Source code",
  },
  { id: "D8", name: "MCP server code", tier: "Medium", weight: 10, looksAt: "Source code" },
  {
    id: "D9",
    name: "Agent / prompt definition file",
    tier: "Medium",
    weight: 10,
    looksAt: "*.prompty, agent.yaml",
  },
  { id: "D10", name: "Model SDK only", tier: "Low", weight: 5, looksAt: "Manifests" },
  {
    id: "D11",
    name: "LLM env-var name",
    tier: "Low",
    weight: 5,
    looksAt: ".env.example, Helm, K8s, CI",
  },
  {
    id: "D12",
    name: "Developer tooling",
    tier: "Info",
    weight: 0,
    looksAt: ".vscode/mcp.json, .cursor/mcp.json, AGENTS.md, CLAUDE.md",
  },
];

const SAMPLE: SourceRepoScanData = {
  detectors: REPO_DETECTORS,
  connectedHosts: [
    { host: "GitHub", organization: "acme-corp", repositories: 128 },
    { host: "Azure DevOps", organization: "acme-platform", repositories: 34 },
  ],
  lastScanAt: "2026-11-09T08:30:00Z",
  suspects: [
    {
      id: "s1",
      repository: "acme-corp/support-assistant",
      host: "GitHub",
      servicePath: "services/assistant",
      commitSha: "4f1c2ab",
      score: 95,
      verdict: "Agent",
      hits: [
        {
          detectorId: "D1",
          path: "services/assistant/pyproject.toml",
          line: 14,
          symbol: "langgraph",
        },
        {
          detectorId: "D2",
          path: "services/assistant/app/graph.py",
          line: 3,
          symbol: "from langgraph.graph import StateGraph",
        },
        {
          detectorId: "D4",
          path: "services/assistant/app/tools.py",
          line: 21,
          symbol: "MultiServerMCPClient",
        },
      ],
      secretFindings: [{ type: "OpenAI API key", path: "services/assistant/.env", line: 2 }],
      lastCommitAt: "2026-11-07T16:12:00Z",
      lastCommitter: "dev-alice",
    },
    {
      id: "s2",
      repository: "acme-corp/invoice-bot",
      host: "GitHub",
      servicePath: ".",
      commitSha: "9b0e771",
      score: 40,
      verdict: "Possible agent",
      hits: [
        { detectorId: "D7", path: "bot/handler.ts", line: 58, symbol: "tools: [lookupInvoice]" },
        { detectorId: "D9", path: "prompts/invoice.prompty", line: 1, symbol: "invoice.prompty" },
        { detectorId: "D10", path: "package.json", line: 22, symbol: "openai" },
        { detectorId: "D11", path: ".env.example", line: 4, symbol: "OPENAI_API_KEY" },
      ],
      secretFindings: [],
      lastCommitAt: "2026-10-28T10:02:00Z",
      lastCommitter: "dev-bob",
    },
    {
      id: "s3",
      repository: "acme-platform/reporting",
      host: "Azure DevOps",
      servicePath: "jobs/summarize",
      commitSha: "c33d9e0",
      score: 10,
      verdict: "LLM consumer",
      hits: [
        {
          detectorId: "D10",
          path: "jobs/summarize/requirements.txt",
          line: 7,
          symbol: "anthropic",
        },
        { detectorId: "D11", path: "deploy/values.yaml", line: 31, symbol: "ANTHROPIC_API_KEY" },
        { detectorId: "D12", path: "AGENTS.md", line: 1, symbol: "AGENTS.md" },
      ],
      secretFindings: [],
      lastCommitAt: "2026-09-30T12:45:00Z",
      lastCommitter: "dev-carol",
    },
  ],
};

/** Sample data for the preview; returns a copy so callers cannot change it. */
export function getSourceRepoScanData(): SourceRepoScanData {
  return structuredClone(SAMPLE);
}

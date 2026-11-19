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

import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import type {
  ShadowAgentSummary,
  ShadowConnectionResponse,
  ShadowFindingDetail,
  ShadowFindingListQuery,
  ShadowFindingSummary,
  ShadowProviderSpec,
  ShadowRunnerScanResponse,
  ShadowScanResponse,
} from "@agent-management-platform/types";

const finding = (over: Partial<ShadowFindingSummary> = {}): ShadowFindingSummary => ({
  id: "f1",
  connectionId: "c1",
  connectionName: "OpenAI prod",
  provider: "openai",
  kind: "platform_agent",
  externalId: "agent_abc",
  displayName: "Support Bot",
  scope: "proj_1",
  state: "active",
  ownerHint: {},
  firstSeenAt: "2026-11-01T00:00:00Z",
  lastSeenAt: "2026-11-10T00:00:00Z",
  present: true,
  foundBy: [{ findingId: "f1", connectionId: "c1", connectionName: "OpenAI prod", present: true }],
  onboarded: false,
  ...over,
});

const detail = (over: Partial<ShadowFindingDetail> = {}): ShadowFindingDetail => ({
  ...finding(),
  attributes: { model: "gpt-4.1", project: "Support" },
  related: [],
  signals: [
    {
      code: "openai.agent_object",
      name: "Agent object",
      source: "GET /v1/agents",
      condition: "GET /v1/agents (OpenAI-Beta: agents=v1, project key): an item in data",
      observed: { toolCount: 2 },
      detectedAt: "2026-11-10T00:00:00Z",
    },
  ],
  ...over,
});

const connection = (over: Partial<ShadowConnectionResponse> = {}): ShadowConnectionResponse => ({
  id: "c1",
  name: "OpenAI prod",
  provider: "openai",
  endpoints: {},
  settings: {},
  secretHints: {},
  endpointWarnings: [],
  createdBy: "",
  createdAt: "2026-11-01T00:00:00Z",
  updatedAt: "2026-11-01T00:00:00Z",
  ...over,
});

const scan = (over: Partial<ShadowScanResponse> = {}): ShadowScanResponse => ({
  id: "s1",
  status: "succeeded",
  connectionIds: ["c1"],
  results: [
    {
      connectionId: "c1",
      connectionName: "OpenAI prod",
      provider: "openai",
      outcome: "ok",
      findingsCount: 3,
      calls: [],
    },
  ],
  findingsCount: 3,
  requestedBy: "",
  createdAt: "2026-11-10T00:00:00Z",
  ...over,
});

const hoisted = vi.hoisted(() => ({
  findings: [] as ShadowFindingSummary[],
  findingsError: null as unknown,
  queries: [] as ShadowFindingListQuery[],
  scopes: new Set<string>(),
  enforced: true,
  detail: null as ShadowFindingDetail | null,
  summary: undefined as ShadowAgentSummary | undefined,
  connections: [] as ShadowConnectionResponse[],
  scans: [] as ShadowScanResponse[],
  scanDetail: null as ShadowScanResponse | null,
  scanIds: [] as (string | undefined)[],
  scanLoading: false,
  scanError: null as unknown,
  createAgent: vi.fn(),
  onboard: vi.fn(),
  start: vi.fn(),
  clear: vi.fn(),
  deleteAgent: vi.fn(),
  deleteAll: vi.fn(),
  test: vi.fn(),
  generate: vi.fn(),
  resetGenerate: vi.fn(),
  addConfirmation: vi.fn(),
  providers: [] as ShadowProviderSpec[],
  createConnection: vi.fn(),
  updateConnection: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useShadowFindings: (_org: string | undefined, query: ShadowFindingListQuery) => {
    hoisted.queries.push(query);
    return {
      data: { findings: hoisted.findings, total: hoisted.findings.length, limit: 50, offset: 0 },
      isLoading: false,
      error: hoisted.findingsError,
    };
  },
  useShadowFinding: () => ({ data: hoisted.detail, isLoading: false, error: null }),
  useOnboardShadowFinding: () => ({ mutate: hoisted.onboard, isPending: false }),
  useCreateAgent: () => ({ mutate: hoisted.createAgent, isPending: false }),
  useListProjects: () => ({
    data: { projects: [{ name: "default", displayName: "Default" }] },
    isLoading: false,
  }),
  useDeleteShadowFinding: () => ({ mutate: hoisted.deleteAgent, isPending: false }),
  useDeleteShadowFindings: () => ({ mutate: hoisted.deleteAll, isPending: false }),
  useTokenScopes: () => ({ enforced: hoisted.enforced, resolved: true, scopes: hoisted.scopes }),
  useShadowSummary: () => ({ data: hoisted.summary, isLoading: false, error: null }),
  useShadowProviders: () => ({
    data: hoisted.providers.length > 0 ? { providers: hoisted.providers, signals: [] } : undefined,
    isLoading: false,
  }),
  useShadowConnection: () => ({ data: undefined, isLoading: false }),
  useCreateShadowConnection: () => ({ mutate: hoisted.createConnection, isPending: false }),
  useUpdateShadowConnection: () => ({ mutate: hoisted.updateConnection, isPending: false }),
  useShadowConnections: () => ({
    data: { connections: hoisted.connections },
    isLoading: false,
    error: null,
  }),
  useShadowScans: () => ({ data: { scans: hoisted.scans }, isLoading: false, error: null }),
  useShadowScan: (_org: string | undefined, id: string | undefined) => {
    hoisted.scanIds.push(id);
    const data =
      hoisted.scanDetail?.id === id ? hoisted.scanDetail : hoisted.scans.find((s) => s.id === id);
    return {
      data: id ? data : undefined,
      isLoading: !!id && hoisted.scanLoading,
      error: id ? hoisted.scanError : null,
    };
  },
  useClearShadowScans: () => ({ mutate: hoisted.clear, isPending: false }),
  useStartShadowScan: () => ({ mutate: hoisted.start, isPending: false }),
  useDeleteShadowConnection: () => ({ mutate: vi.fn(), isPending: false }),
  useTestShadowConnection: () => ({ mutate: hoisted.test, isPending: false }),
  useCreateShadowRunnerScan: () => ({
    mutate: hoisted.generate,
    reset: hoisted.resetGenerate,
    isPending: false,
  }),
  isShadowScanActive: (s: string | undefined) => s === "queued" || s === "running",
  useTrack: () => ({ track: vi.fn() }),
  ConsoleAction: {},
}));

vi.mock("@agent-management-platform/shared-component", async () => {
  const React = await import("react");
  return {
    ResourceListShell: ({
      isEmpty,
      error,
      emptyState,
      toolbar,
      children,
    }: {
      isEmpty?: boolean;
      error?: Error;
      emptyState: { title: string };
      toolbar?: React.ReactNode;
      children: React.ReactNode;
    }) =>
      React.createElement(
        "div",
        null,
        toolbar,
        error ? error.message : isEmpty ? emptyState.title : children,
      ),
    isListEmpty: ({ isLoading, count }: { isLoading: boolean; count: number }) =>
      !isLoading && count === 0,
    formatRelativeTime: () => "2 days ago",
    CodeBlock: ({ code }: { code: string }) => React.createElement("pre", null, code),
    useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
    useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  };
});

import { FindingsTab } from "./subComponents/FindingsTab";
import { FindingDetail, relationLabel } from "./subComponents/FindingDetail";
import { OnboardFinding } from "./subComponents/OnboardFinding";
import { OverviewPanel } from "./subComponents/OverviewPanel";
import { RunScanButton } from "./subComponents/RunScanButton";
import { ConnectionsTab, ScopedKeysChip } from "./subComponents/ConnectionsTab";
import { ScanDetailsDrawer, ScansTab } from "./subComponents/ScansTab";
import { ShadowAgentDiscovery } from "./ShadowAgentDiscovery";
import { AnalyzeSection } from "./subComponents/AnalyzeSection";
import { ScanStatusChip } from "./shadowUi";
import { SourceScanGuide } from "./subComponents/SourceScanGuide";
import { DocLinks } from "./subComponents/PermissionsPanel";
import { GenerateCommandDialog } from "./subComponents/GenerateCommandDialog";
import { ConnectionEditor } from "./subComponents/ConnectionEditor";

function ShowLocation() {
  const l = useLocation();
  return <div data-testid="location">{`${l.pathname}${l.search}`}</div>;
}

const router = (ui: React.ReactNode, path = "/organizations/acme/shadow-agent-discovery") =>
  render(
    <MemoryRouter
      initialEntries={[path]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      {ui}
      <ShowLocation />
    </MemoryRouter>,
  );

const page = (path = "/organizations/acme/shadow-agent-discovery") =>
  router(
    <Routes>
      <Route
        path="/organizations/:orgId/shadow-agent-discovery/*"
        element={<ShadowAgentDiscovery />}
      />
    </Routes>,
    path,
  );

const lastQuery = () => hoisted.queries[hoisted.queries.length - 1];

/** Opens a MUI select by its accessible name and returns its option labels. */
const openSelect = (name: string) => {
  fireEvent.mouseDown(screen.getByRole("combobox", { name }));
  return within(screen.getByRole("listbox"))
    .getAllByRole("option")
    .map((o) => o.textContent);
};

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.findings = [];
  hoisted.findingsError = null;
  hoisted.queries = [];
  hoisted.scopes = new Set(["amp:shadow-agent:read", "amp:shadow-agent:manage"]);
  hoisted.enforced = true;
  hoisted.detail = null;
  hoisted.summary = undefined;
  hoisted.connections = [];
  hoisted.scans = [];
  hoisted.scanDetail = null;
  hoisted.scanIds = [];
  hoisted.scanLoading = false;
  hoisted.scanError = null;
  hoisted.providers = [];
});

describe("ShadowAgentDiscovery page", () => {
  it("explains the access needed instead of showing the page", () => {
    hoisted.scopes = new Set();
    page();
    expect(screen.getByText("You don't have access to Shadow Agent Discovery")).toBeInTheDocument();
  });

  it("has the Overview, Unregistered Agents, Scans and Platforms tabs", () => {
    page();
    expect(screen.getByRole("heading", { name: "Shadow Agent Discovery" })).toBeInTheDocument();
    const tabs = screen.getAllByRole("tab").map((t) => t.textContent);
    expect(tabs).toEqual(["Overview", "Unregistered Agents", "Scans", "Platforms"]);
    expect(screen.queryByRole("tab", { name: "Connections" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Findings" })).not.toBeInTheDocument();
  });

  it("opens Unregistered Agents from the URL and keeps the platform filter there", () => {
    hoisted.findings = [finding()];
    page("/organizations/acme/shadow-agent-discovery?tab=agents&platform=bedrock");
    expect(screen.getByRole("tab", { name: "Unregistered Agents" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(lastQuery()).toMatchObject({ provider: "bedrock" });
    expect(lastQuery().scanId).toBeUndefined();
    openSelect("Platform");
    fireEvent.click(screen.getByRole("option", { name: "Anthropic" }));
    expect(screen.getByTestId("location")).toHaveTextContent("tab=agents&platform=anthropic");
    expect(lastQuery()).toMatchObject({ provider: "anthropic" });
  });

  it("switches tabs through the URL", () => {
    page();
    fireEvent.click(screen.getByRole("tab", { name: "Platforms" }));
    expect(screen.getByTestId("location")).toHaveTextContent("?tab=platforms");
    fireEvent.click(screen.getByRole("tab", { name: "Unregistered Agents" }));
    expect(screen.getByTestId("location")).toHaveTextContent("?tab=agents");
  });
});

describe("OverviewPanel", () => {
  const props = {
    orgName: "acme",
    canManage: true,
    onConnect: vi.fn(),
    onOpenScans: vi.fn(),
    onOpenPlatforms: vi.fn(),
    onOpenAgents: vi.fn(),
  };

  it("asks to add a platform first, with the steps to follow", () => {
    hoisted.summary = { totalFindings: 0, byProvider: {}, onboarded: 0, connections: 0 };
    router(<OverviewPanel {...props} />);
    expect(screen.getByText("Add a platform to start")).toBeInTheDocument();
    for (const step of [
      "Add a platform",
      "Test the connection",
      "Run a scan",
      "Onboard the unregistered agents",
    ]) {
      expect(screen.getAllByText(step).length).toBeGreaterThan(0);
    }
    fireEvent.click(screen.getAllByRole("button", { name: "Add platform" })[0]);
    expect(props.onConnect).toHaveBeenCalled();
  });

  it("shows the agents found, by platform, and the latest scan, without scores", () => {
    hoisted.summary = {
      totalFindings: 5,
      byProvider: { openai: 3, bedrock: 2 },
      onboarded: 1,
      connections: 2,
      latestScan: scan({ connectionIds: ["c1", "c2"] }),
    };
    router(<OverviewPanel {...props} />);
    expect(screen.getByText("Unregistered agents")).toBeInTheDocument();
    expect(screen.getByText("Onboarded")).toBeInTheDocument();
    expect(screen.getByText("Platforms connected")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText(/2 platforms scanned/)).toBeInTheDocument();
    expect(screen.queryByText(/score/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/verdict/i)).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Amazon Bedrock · 2"));
    expect(props.onOpenAgents).toHaveBeenCalledWith("bedrock");
    fireEvent.click(screen.getByRole("button", { name: "All scans" }));
    expect(props.onOpenScans).toHaveBeenCalled();
    fireEvent.click(screen.getAllByRole("button", { name: "View agents" })[0]);
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/shadow-agent-discovery/scans/s1",
    );
  });
});

describe("FindingsTab", () => {
  it("lists each agent with its platform, source, state and onboarding", () => {
    hoisted.findings = [
      finding(),
      finding({
        id: "f2",
        displayName: "triage",
        provider: "bedrock",
        kind: "runtime",
        scope: "us-east-1",
        state: "READY",
        onboarded: true,
        linkedProject: "default",
        linkedAgent: "triage",
        foundBy: [
          { findingId: "f2", connectionId: "c2", connectionName: "AWS prod", present: true },
          { findingId: "f3", connectionId: "c3", connectionName: "AWS audit", present: true },
        ],
      }),
    ];
    router(<FindingsTab orgName="acme" />);
    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent);
    expect(headers).toEqual(["Agent", "Platform", "Source", "State", "Last seen", "Onboarding"]);
    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.getByText("AgentCore runtime · us-east-1")).toBeInTheDocument();
    expect(screen.getByText("Amazon Bedrock")).toBeInTheDocument();
    expect(screen.getByText("AWS prod, AWS audit")).toBeInTheDocument();
    expect(screen.getByText("READY")).toBeInTheDocument();
    expect(screen.getByText("Onboarded")).toBeInTheDocument();
    expect(screen.getByText("Not onboarded")).toBeInTheDocument();
    expect(screen.queryByText(/Reviewed|Dismissed|Status/)).not.toBeInTheDocument();
    expect(screen.queryByText("No longer listed")).not.toBeInTheDocument();
  });

  it("shows the platform ID of distinct agents that share a name", () => {
    hoisted.findings = [
      finding({ id: "a", externalId: "agent_one", displayName: "Docs", scope: "proj_1" }),
      finding({ id: "b", externalId: "agent_two", displayName: "Docs", scope: "proj_1" }),
      finding({ id: "c", externalId: "agent_three", displayName: "Other", scope: "proj_1" }),
    ];
    router(<FindingsTab orgName="acme" />);
    expect(screen.getByText(/· agent_one$/)).toBeInTheDocument();
    expect(screen.getByText(/· agent_two$/)).toBeInTheDocument();
    expect(screen.queryByText(/agent_three/)).not.toBeInTheDocument();
  });

  it("has no delete actions without the manage permission, or for a scan's agents", () => {
    hoisted.findings = [finding()];
    const { unmount } = router(<FindingsTab orgName="acme" />);
    expect(screen.queryByRole("button", { name: "Delete all" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete Support Bot" })).not.toBeInTheDocument();
    unmount();
    router(<FindingsTab orgName="acme" scanId="s1" canManage />);
    expect(screen.queryByRole("button", { name: "Delete all" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete Support Bot" })).not.toBeInTheDocument();
  });

  it("deletes one agent after confirming, without opening it", () => {
    hoisted.findings = [finding()];
    router(<FindingsTab orgName="acme" canManage />);
    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent);
    expect(headers).toEqual([
      "Agent",
      "Platform",
      "Source",
      "State",
      "Last seen",
      "Onboarding",
      "Actions",
    ]);
    fireEvent.click(screen.getByRole("button", { name: "Delete Support Bot" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/");
    expect(screen.getByTestId("location")).not.toHaveTextContent("/agents/");
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.title).toBe("Delete Support Bot?");
    expect(opts.description).toContain("for every platform that found it");
    expect(opts.description).toContain("a later scan lists the agent again");
    expect(hoisted.deleteAgent).not.toHaveBeenCalled();
    opts.onConfirm();
    expect(hoisted.deleteAgent).toHaveBeenCalledWith("f1");
  });

  it("deletes every agent after confirming, whatever the filters show", () => {
    hoisted.findings = [finding()];
    router(<FindingsTab orgName="acme" canManage />);
    fireEvent.click(screen.getByRole("button", { name: "Delete all" }));
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.title).toBe("Delete all unregistered agents?");
    expect(opts.description).toContain("on every platform, whatever the filters show");
    expect(opts.description).toContain("including the agents their platforms no longer list");
    expect(opts.description).toContain("Platforms and scan history are kept");
    expect(opts.confirmButtonText).toBe("Delete all");
    expect(hoisted.deleteAll).not.toHaveBeenCalled();
    opts.onConfirm();
    expect(hoisted.deleteAll).toHaveBeenCalledTimes(1);
  });

  it("disables Delete all when there is no agent", () => {
    hoisted.findings = [];
    router(<FindingsTab orgName="acme" canManage />);
    expect(screen.getByRole("button", { name: "Delete all" })).toBeDisabled();
  });

  it("marks an agent its platform no longer lists, and opens an agent", () => {
    hoisted.findings = [finding({ present: false })];
    router(<FindingsTab orgName="acme" scanId="s1" />);
    expect(lastQuery()).toMatchObject({ scanId: "s1" });
    expect(screen.getByText("No longer listed")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Support Bot"));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/shadow-agent-discovery/agents/f1",
    );
  });

  it("filters by the agent platforms, the source hosts and onboarding", () => {
    hoisted.findings = [finding()];
    router(<FindingsTab orgName="acme" />);
    expect(openSelect("Platform")).toEqual([
      "All platforms",
      "OpenAI",
      "Anthropic",
      "Amazon Bedrock",
      "Google Agent Platform",
      "Microsoft Foundry",
      "GitHub",
      "Azure DevOps",
    ]);
    fireEvent.click(screen.getByRole("option", { name: "Microsoft Foundry" }));
    expect(lastQuery()).toMatchObject({ provider: "foundry", offset: 0 });
    expect(openSelect("Onboarding")).toEqual(["All agents", "Not onboarded", "Onboarded"]);
    fireEvent.click(screen.getByRole("option", { name: "Not onboarded" }));
    expect(lastQuery()).toMatchObject({ provider: "foundry", onboarded: "false" });
  });

  it("shows the empty state, not an error, when there are no agents", () => {
    router(<FindingsTab orgName="acme" />);
    expect(screen.getByText("No unregistered agents yet")).toBeInTheDocument();
  });

  it("shows only the error when loading fails", () => {
    hoisted.findingsError = new Error("boom");
    router(<FindingsTab orgName="acme" />);
    expect(screen.getByText(/Couldn't load the agents/)).toBeInTheDocument();
    expect(screen.queryByText("No unregistered agents yet")).not.toBeInTheDocument();
  });
});

describe("FindingDetail", () => {
  const renderDetail = (canManage = true) =>
    router(
      <Routes>
        <Route
          path="/organizations/:orgId/shadow-agent-discovery/agents/:findingId"
          element={
            <FindingDetail
              orgName="acme"
              canManage={canManage}
              listPath="/organizations/acme/shadow-agent-discovery"
            />
          }
        />
      </Routes>,
      "/organizations/acme/shadow-agent-discovery/agents/f1",
    );

  it("shows the details, source and evidence, and offers onboarding", () => {
    hoisted.detail = detail({
      attributes: {
        model: "gpt-4.1",
        tools: [{ type: "function", name: "lookup_order" }],
        mcpServers: [{ label: "github", origin: "https://mcp.example.com" }],
      },
    });
    renderDetail();
    expect(screen.getByRole("heading", { name: /^Support Bot/ })).toBeInTheDocument();
    expect(screen.getByText("Agent on OpenAI")).toBeInTheDocument();
    expect(screen.getByText("Not onboarded")).toBeInTheDocument();
    for (const section of ["Details", "Source", "Activity", "Evidence"]) {
      expect(screen.getByText(section)).toBeInTheDocument();
    }
    expect(screen.getByText("Agent object")).toBeInTheDocument();
    expect(screen.getByText(/an item in data/)).toBeInTheDocument();
    expect(screen.getByText("toolCount: 2")).toBeInTheDocument();
    expect(screen.getByText("gpt-4.1")).toBeInTheDocument();
    expect(screen.getByText("function · lookup_order")).toBeInTheDocument();
    expect(screen.getByText("github · https://mcp.example.com")).toBeInTheDocument();
    expect(screen.getByText("agent_abc")).toBeInTheDocument();
    expect(screen.getByText("Listed")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Onboard as external agent" })).toHaveAttribute(
      "href",
      "/organizations/acme/shadow-agent-discovery/agents/f1/onboard",
    );
    expect(screen.queryByText(/score|verdict|reviewed|dismiss/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/no longer listed/i)).not.toBeInTheDocument();
    expect(
      screen.getByText("Why this is an agent: the platform listing that returned it."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/can make mistakes/)).not.toBeInTheDocument();
  });

  const codeAgent = (reviewedBy: string, over: Partial<ShadowFindingDetail> = {}) =>
    detail({
      provider: "github",
      kind: "code_agent",
      externalId: "wso2/support-bot:agent.py",
      attributes: { framework: "langgraph", reviewedBy },
      signals: [
        {
          code: "github.d1_agent_dependency",
          name: "Agent framework dependency",
          source: "requirements.txt",
          condition: "A dependency on an agent framework",
          observed: { dependency: "langgraph" },
          detectedAt: "2026-11-10T00:00:00Z",
        },
      ],
      ...over,
    });

  it("explains code evidence and warns that AI review confirmed a code agent", () => {
    hoisted.detail = codeAgent("ai_review_judge");
    renderDetail();
    expect(
      screen.getByText("Why this is an agent: the code in the repository that matched each rule."),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Why this is an agent: the platform listing that returned it."),
    ).toBeNull();
    expect(screen.getByText("requirements.txt")).toBeInTheDocument();
    expect(screen.getByText("Confirmed by")).toBeInTheDocument();
    expect(screen.getByText("AI review and LLM judge")).toBeInTheDocument();
    expect(
      screen.getByText(
        "AI review confirmed this agent. AI review can make mistakes, so check the evidence.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Location in code")).toBeInTheDocument();
    expect(screen.getByText("wso2/support-bot:agent.py")).toBeInTheDocument();
    expect(screen.getAllByText("Repository").length).toBeGreaterThan(0);
    expect(screen.queryByLabelText("Platform ID")).not.toBeInTheDocument();
    expect(screen.queryByText("Scope")).not.toBeInTheDocument();
    expect(screen.getByText(/What the scan command reported/)).toBeInTheDocument();
  });

  it("warns about AI mistakes for a code agent confirmed by AI review alone", () => {
    hoisted.detail = codeAgent("ai_review");
    renderDetail();
    expect(screen.getByText("AI review")).toBeInTheDocument();
    expect(screen.getByText(/AI review can make mistakes/)).toBeInTheDocument();
  });

  it("words a code agent that is no longer found, without platform-only fields", () => {
    hoisted.detail = codeAgent("ai_review", {
      present: false,
      foundBy: [
        { findingId: "f1", connectionId: "c1", connectionName: "WSO2 GitHub", present: false },
        { findingId: "f9", connectionId: "c9", connectionName: "WSO2 mirror", present: true },
      ],
    });
    renderDetail();
    expect(screen.getAllByRole("alert")).toHaveLength(2);
    expect(screen.getByText(/no longer found this agent/)).toBeInTheDocument();
    expect(screen.queryByText(/no longer listed this agent/)).not.toBeInTheDocument();
    expect(screen.getByText("No longer found")).toBeInTheDocument();
    expect(screen.queryByText("No longer listed")).not.toBeInTheDocument();
    expect(screen.getByText("Found")).toBeInTheDocument();
    expect(screen.getByText(/found through 2 connections/)).toBeInTheDocument();
    expect(screen.queryByText("State at the platform")).not.toBeInTheDocument();
    expect(screen.queryByText("Created at the platform")).not.toBeInTheDocument();
  });

  it("shows a rule-proven code agent without the AI warning", () => {
    hoisted.detail = codeAgent("rules");
    renderDetail();
    expect(screen.getByText("Confirmed by")).toBeInTheDocument();
    expect(screen.getByText("Rules")).toBeInTheDocument();
    expect(screen.queryByText(/can make mistakes/)).not.toBeInTheDocument();
  });

  it("links an onboarded agent to the registered agent", () => {
    hoisted.detail = detail({
      onboarded: true,
      linkedProject: "default",
      linkedAgent: "support-bot",
      onboardedAt: "2026-11-12T00:00:00Z",
    });
    renderDetail();
    expect(screen.getByText("Onboarded")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "default / support-bot" })).toHaveAttribute(
      "href",
      "/organizations/acme/projects/default/agents/support-bot",
    );
    expect(screen.getByRole("link", { name: "Open registered agent" })).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Onboard as external agent" }),
    ).not.toBeInTheDocument();
  });

  it("notes an agent found through several connections, and its owner when listed", () => {
    hoisted.detail = detail({
      ownerHint: { email: "alice@acme.com", source: "GET /v1/agents" },
      foundBy: [
        { findingId: "f1", connectionId: "c1", connectionName: "OpenAI prod", present: true },
        { findingId: "f9", connectionId: "c9", connectionName: "OpenAI audit", present: false },
      ],
    });
    renderDetail();
    expect(screen.getByText("OpenAI audit")).toBeInTheDocument();
    expect(screen.getByText(/found through 2 connections/)).toBeInTheDocument();
    expect(screen.getByText("No longer listed")).toBeInTheDocument();
    expect(screen.getByText(/Created by/)).toBeInTheDocument();
    expect(screen.getByText("alice@acme.com")).toBeInTheDocument();
  });

  it("warns when the platform no longer lists the agent, and shows possible duplicates", () => {
    hoisted.detail = detail({
      present: false,
      related: [
        {
          findingId: "f7",
          relation: "possible_duplicate",
          displayName: "support-bot",
          provider: "foundry",
        },
      ],
    });
    renderDetail();
    expect(screen.getByText(/no longer listed this agent/)).toBeInTheDocument();
    expect(screen.getByText("Possible duplicates")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "support-bot" })).toHaveAttribute(
      "href",
      "/organizations/acme/shadow-agent-discovery/agents/f7",
    );
    expect(relationLabel("possible_duplicate")).toBe("Possible duplicate");
  });

  it("has no onboarding action for a read-only user", () => {
    hoisted.detail = detail();
    renderDetail(false);
    expect(
      screen.queryByRole("link", { name: "Onboard as external agent" }),
    ).not.toBeInTheDocument();
  });
});

describe("RunScanButton", () => {
  beforeEach(() => {
    hoisted.connections = [
      connection(),
      connection({ id: "c2", name: "AWS prod", provider: "bedrock" }),
    ];
  });

  it("scans every platform", () => {
    router(<RunScanButton orgName="acme" />);
    fireEvent.click(screen.getByRole("button", { name: "Run scan" }));
    expect(screen.getByLabelText("All platforms (2)")).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Start scan" }));
    expect(hoisted.start).toHaveBeenCalledWith({}, expect.anything());
  });

  it("scans only the platforms chosen", () => {
    router(<RunScanButton orgName="acme" />);
    fireEvent.click(screen.getByRole("button", { name: "Run scan" }));
    fireEvent.click(screen.getByLabelText("Choose platforms"));
    expect(screen.getByRole("button", { name: "Start scan" })).toBeDisabled();
    fireEvent.click(screen.getByLabelText("AWS prod · Amazon Bedrock"));
    fireEvent.click(screen.getByRole("button", { name: "Start scan" }));
    expect(hoisted.start).toHaveBeenCalledWith({ connectionIds: ["c2"] }, expect.anything());
  });

  it("is disabled while a scan runs or with no platform", () => {
    hoisted.summary = {
      totalFindings: 0,
      byProvider: {},
      onboarded: 0,
      connections: 2,
      latestScan: scan({ status: "running" }),
    };
    const { unmount } = router(<RunScanButton orgName="acme" />);
    expect(screen.getByRole("button", { name: "Scan running…" })).toBeDisabled();
    unmount();
    hoisted.summary = undefined;
    hoisted.connections = [];
    const { unmount: unmountEmpty } = router(<RunScanButton orgName="acme" />);
    expect(screen.getByRole("button", { name: "Run scan" })).toBeDisabled();
    unmountEmpty();
    // Source code repositories are scanned by the scan command, not by Run scan.
    hoisted.connections = [connection({ id: "c2", name: "Repos", provider: "github" })];
    router(<RunScanButton orgName="acme" />);
    expect(screen.getByRole("button", { name: "Run scan" })).toBeDisabled();
  });
});

describe("ConnectionsTab (Platforms)", () => {
  const renderTab = () =>
    router(<ConnectionsTab orgName="acme" canManage onAdd={vi.fn()} onEdit={vi.fn()} />);

  it("lists AI platforms and source code repositories in their own sections", () => {
    hoisted.connections = [
      connection(),
      connection({
        id: "c2",
        name: "Repos",
        provider: "github",
        settings: { organization: "acme", allRepositories: true, privateRepositories: true },
      }),
    ];
    renderTab();
    expect(screen.getByRole("button", { name: "Add platform" })).toBeInTheDocument();
    expect(screen.getByText("AI platforms")).toBeInTheDocument();
    expect(screen.getByText("Source code repositories")).toBeInTheDocument();
    const [ai, source] = screen.getAllByRole("table");
    expect(within(ai).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Name",
      "Platform",
      "Last test",
      "Created",
      "",
    ]);
    expect(within(source).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Name",
      "Host",
      "What it scans",
      "Created",
      "",
    ]);
    expect(within(source).getByText("acme · All repositories · Private and public")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Scan OpenAI prod" }));
    expect(hoisted.start).toHaveBeenCalledWith({ connectionIds: ["c1"] });
    // A source host has no credential to test and no cloud scan.
    expect(screen.queryByRole("button", { name: "Scan Repos" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Test Repos" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit Repos" })).toBeInTheDocument();
  });

  it("opens Generate command from a source code repository row", () => {
    hoisted.connections = [connection({ id: "c2", name: "Repos", provider: "github" })];
    renderTab();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Generate command for Repos" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("Generate scan command: Repos");
  });

  it("shows only the sections that have platforms", () => {
    hoisted.connections = [connection()];
    renderTab();
    expect(screen.getByText("AI platforms")).toBeInTheDocument();
    expect(screen.queryByText("Source code repositories")).not.toBeInTheDocument();
  });

  it("disables Scan while a scan runs", () => {
    hoisted.connections = [connection()];
    hoisted.summary = {
      totalFindings: 0,
      byProvider: {},
      onboarded: 0,
      connections: 1,
      latestScan: scan({ status: "queued" }),
    };
    renderTab();
    expect(screen.getByRole("button", { name: "Scan OpenAI prod" })).toBeDisabled();
  });

  it("deletes a platform after saying what is removed", () => {
    hoisted.connections = [
      connection(),
      connection({ id: "c2", name: "Repos", provider: "github" }),
    ];
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Delete OpenAI prod" }));
    expect(hoisted.addConfirmation).toHaveBeenLastCalledWith(
      expect.objectContaining({
        description: expect.stringContaining("every agent this platform found"),
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Delete Repos" }));
    expect(hoisted.addConfirmation).toHaveBeenLastCalledWith(
      expect.objectContaining({
        description: expect.stringContaining("Every agent found in these repositories"),
      }),
    );
  });
});

describe("ScopedKeysChip", () => {
  it("counts the keys OpenAI and Anthropic platforms list agents with", () => {
    const { rerender, container } = render(
      <ScopedKeysChip
        provider="openai"
        secretHints={{ "scopedKeys.a": "x", "scopedKeys.b": "y", apiKey: "sk-a…" }}
      />,
    );
    expect(screen.getByText("2 project keys")).toBeInTheDocument();
    expect(screen.queryByText(/Admin/)).not.toBeInTheDocument();
    rerender(<ScopedKeysChip provider="anthropic" secretHints={{ "scopedKeys.w": "x" }} />);
    expect(screen.getByText("1 workspace key")).toBeInTheDocument();
    rerender(<ScopedKeysChip provider="bedrock" secretHints={{}} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("ScansTab", () => {
  it("lists scans with the platforms scanned and the agents found", () => {
    hoisted.scans = [scan()];
    router(<ScansTab orgName="acme" canManage />);
    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent);
    expect(headers).toEqual(["Started", "Status", "Platforms scanned", "Agents found", ""]);
    expect(screen.queryByText(/History read|days of history/)).not.toBeInTheDocument();
    expect(screen.getAllByText("OpenAI prod").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: /View agents of the scan/ }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/shadow-agent-discovery/scans/s1",
    );
  });

  it("clears the finished scans after saying the agents are kept", () => {
    hoisted.scans = [scan()];
    router(<ScansTab orgName="acme" canManage />);
    fireEvent.click(screen.getByRole("button", { name: "Clear scans" }));
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.title).toBe("Clear all scans?");
    expect(opts.description).toContain("Unregistered Agents and their evidence are kept");
    opts.onConfirm();
    expect(hoisted.clear).toHaveBeenCalledTimes(1);
  });

  const github = {
    connectionId: "c2",
    connectionName: "GitHub acme",
    provider: "github",
    outcome: "ok",
    findingsCount: 1,
    calls: [],
  };
  const openGithubScan = () => {
    router(<ScansTab orgName="acme" canManage />);
    fireEvent.click(screen.getByText("GitHub acme"));
    return screen.getByRole("dialog", { name: "Scan details" });
  };

  it("opens a scan with its detail, which has the repositories", () => {
    hoisted.scans = [scan({ id: "s2", connectionIds: ["c2"], results: [github] })];
    hoisted.scanDetail = scan({
      id: "s2",
      connectionIds: ["c2"],
      results: [{ ...github, scannedRepositories: ["acme/api"] }],
    });
    router(<ScansTab orgName="acme" canManage />);
    expect(hoisted.scanIds.filter(Boolean)).toEqual([]);
    fireEvent.click(screen.getByText("GitHub acme"));
    expect(hoisted.scanIds).toContain("s2");
    const drawer = screen.getByRole("dialog", { name: "Scan details" });
    expect(within(drawer).getByText("1 scanned")).toBeInTheDocument();
  });

  it("shows the list item, not a detail of another scan", () => {
    hoisted.scans = [scan({ id: "s2", connectionIds: ["c2"], results: [github] })];
    hoisted.scanDetail = scan({
      id: "s9",
      connectionIds: ["c2"],
      results: [{ ...github, scannedRepositories: ["acme/other"] }],
    });
    const drawer = openGithubScan();
    expect(within(drawer).getByText("GitHub acme")).toBeInTheDocument();
    expect(within(drawer).queryByText("Repositories")).not.toBeInTheDocument();
  });

  it("says the repositories are loading while the detail loads", () => {
    hoisted.scans = [scan({ id: "s2", connectionIds: ["c2"], results: [github] })];
    hoisted.scanLoading = true;
    const drawer = openGithubScan();
    expect(within(drawer).getByText("Loading repositories…")).toBeInTheDocument();
  });

  it("says the repositories could not be loaded when the detail fails", () => {
    hoisted.scans = [scan({ id: "s2", connectionIds: ["c2"], results: [github] })];
    hoisted.scanError = new Error("boom");
    const drawer = openGithubScan();
    expect(
      within(drawer).getByText("Couldn't load the repositories. Close and reopen to try again."),
    ).toBeInTheDocument();
  });

  it("says nothing about repositories for an AI platform while the detail loads", () => {
    hoisted.scans = [scan()];
    hoisted.scanLoading = true;
    router(<ScansTab orgName="acme" canManage />);
    fireEvent.click(screen.getAllByText("OpenAI prod")[0]);
    const drawer = screen.getByRole("dialog", { name: "Scan details" });
    expect(within(drawer).queryByText("Loading repositories…")).not.toBeInTheDocument();
  });

  it("keeps a pending scan when clearing", () => {
    hoisted.scans = [scan({ id: "s2", status: "pending" })];
    router(<ScansTab orgName="acme" canManage />);
    expect(screen.getByRole("button", { name: "Clear scans" })).toBeDisabled();
  });
});

describe("ScanDetailsDrawer", () => {
  const sourceScan = () =>
    scan({
      connectionIds: ["c2"],
      results: [
        {
          connectionId: "c2",
          connectionName: "GitHub acme",
          provider: "github",
          outcome: "partial",
          findingsCount: 2,
          calls: [],
          scannedRepositories: ["acme/api", "acme/web"],
          failedRepositories: [{ name: "acme/big", message: "clone timed out" }],
        },
      ],
    });

  it("shows the failed repositories with their reasons", () => {
    render(<ScanDetailsDrawer scan={sourceScan()} onClose={() => undefined} />);
    expect(screen.getByText("Repositories")).toBeInTheDocument();
    expect(screen.getByText("2 scanned, 1 failed")).toBeInTheDocument();
    const failed = screen.getByRole("list", { name: "Failed repositories" });
    expect(within(failed).getByText("acme/big")).toBeInTheDocument();
    expect(within(failed).getByText("clone timed out")).toBeInTheDocument();
  });

  it("lists the scanned repositories on request", () => {
    render(<ScanDetailsDrawer scan={sourceScan()} onClose={() => undefined} />);
    expect(screen.queryByText("acme/api")).not.toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: "Show 2 scanned repositories" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    const list = screen.getByRole("list", { name: "Scanned repositories" });
    expect(toggle).toHaveAttribute("aria-controls", list.id);
    expect(within(list).getAllByRole("listitem").map((l) => l.textContent)).toEqual([
      "acme/api",
      "acme/web",
    ]);
    fireEvent.click(screen.getByRole("button", { name: "Hide scanned repositories" }));
    expect(screen.queryByText("acme/api")).not.toBeInTheDocument();
  });

  it("shows no repositories for an AI platform", () => {
    render(<ScanDetailsDrawer scan={scan()} onClose={() => undefined} />);
    expect(screen.getByText("OpenAI prod")).toBeInTheDocument();
    expect(screen.queryByText("Repositories")).not.toBeInTheDocument();
  });
});

describe("ScanStatusChip", () => {
  it("shows a scan waiting for its scan command as Pending", () => {
    render(<ScanStatusChip status="pending" />);
    expect(screen.getByText("Pending")).toBeInTheDocument();
  });
});

describe("AnalyzeSection", () => {
  const statuses = [
    {
      capability: {
        key: "bedrock.registry",
        title: "AWS Agent Registry agent records",
        description: "Agent records in each registry.",
        signals: ["bedrock.registry_agent_record"],
        needs: ["scanAgentRegistry"],
      },
      available: false,
      missing: ["Scan the AWS Agent Registry"],
    },
    {
      capability: {
        key: "bedrock.agentcore",
        title: "AgentCore runtimes",
        description: "Agent runtimes in each region.",
        signals: ["bedrock.agentcore_runtime"],
        needs: ["scanAgentCore"],
      },
      available: true,
      missing: [],
    },
  ];

  it("simply says what an agent platform's scan lists", () => {
    render(<AnalyzeSection providerName="Amazon Bedrock" statuses={statuses} />);
    expect(screen.getByText("What does Agent Manager list?")).toBeInTheDocument();
    expect(
      screen.getByText(
        "AWS Agent Registry agent records, AgentCore runtimes, with read-only calls.",
      ),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "See what it lists" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("What Agent Manager lists");
    expect(dialog).toHaveTextContent("A scan calls only Amazon Bedrock's agent listings");
    expect(dialog).toHaveTextContent("Agent records in each registry.");
    expect(dialog).toHaveTextContent("Agent runtimes in each region.");
    expect(dialog).not.toHaveTextContent(/Available|Not available|Needs:/);
  });

  it("says what the scan looks for in a source repository host", () => {
    render(<AnalyzeSection providerName="GitHub" statuses={statuses} sourceHost />);
    expect(screen.getByText("What does the scan look for?")).toBeInTheDocument();
    expect(screen.getByText("1 of 2 with these options.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "See what it looks for" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("The scan runs on your machine");
  });

  it("renders nothing for a platform without capabilities", () => {
    const { container } = render(<AnalyzeSection providerName="X" statuses={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});

const sourceHostSpec = (over: Partial<ShadowProviderSpec> = {}): ShadowProviderSpec => ({
  provider: "github",
  displayName: "GitHub",
  description: "",
  endpoints: [],
  fields: [],
  permissions: [
    {
      title: "Fine-grained personal access token (read-only), for private repositories",
      summary: "The scan command asks for it in your terminal when it runs.",
      items: ["Repository permissions: Contents: Read-only."],
    },
  ],
  docs: ["https://docs.github.com/en/authentication"],
  sourceHost: true,
  capabilities: [],
  ...over,
});

describe("SourceScanGuide", () => {
  it("explains the GitHub scan in a dialog, with worded documentation links", () => {
    render(<SourceScanGuide spec={sourceHostSpec()} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "How it works" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("How GitHub scanning works");
    expect(dialog).toHaveTextContent("Save the organization and repositories here");
    expect(dialog).toHaveTextContent("when private repositories are included");
    expect(dialog).toHaveTextContent("Repository permissions: Contents: Read-only.");
    expect(dialog).toHaveTextContent("Your Git token, your LLM key and your code");
    const link = within(dialog).getByRole("link", { name: "GitHub documentation" });
    expect(link).toHaveAttribute("href", "https://docs.github.com/en/authentication");
    expect(link).toHaveAttribute("target", "_blank");
    expect(dialog).not.toHaveTextContent("docs.github.com");
    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
  });

  it("says projects, and an always-needed token, for Azure DevOps", () => {
    render(
      <SourceScanGuide
        spec={sourceHostSpec({ provider: "azure_devops", displayName: "Azure DevOps" })}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "How it works" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("Save the organization and projects here");
    expect(dialog).not.toHaveTextContent("when private repositories are included");
  });
});

describe("DocLinks", () => {
  it("numbers several links of one platform instead of showing their addresses", () => {
    render(
      <DocLinks
        spec={sourceHostSpec({
          displayName: "Amazon Bedrock",
          docs: ["https://docs.aws.amazon.com/a", "https://docs.aws.amazon.com/b"],
        })}
      />,
    );
    expect(
      screen.getByRole("link", { name: "Amazon Bedrock documentation (1 of 2)" }),
    ).toHaveAttribute("href", "https://docs.aws.amazon.com/a");
    expect(screen.getByRole("link", { name: "Amazon Bedrock documentation (2 of 2)" })).toBeVisible();
    expect(screen.queryByText(/docs\.aws\.amazon\.com/)).not.toBeInTheDocument();
  });
});

type RunnerMutateOptions = {
  onSuccess: (r: ShadowRunnerScanResponse) => void;
  onError: (e: unknown) => void;
};
const issuedScan = (over: Partial<ShadowRunnerScanResponse> = {}): ShadowRunnerScanResponse => ({
  scanId: "s9",
  token: "tok-1",
  expiresAt: "2026-11-17T10:00:00Z",
  ampUrl: "https://amp.example.com",
  image: "ghcr.io/wso2/scanner:1.0",
  ...over,
});

describe("GenerateCommandDialog", () => {
  type MutateOptions = RunnerMutateOptions;
  const target = connection({
    id: "c2",
    name: "WSO2 GitHub",
    provider: "github",
    settings: { organization: "wso2", allRepositories: true },
  });
  const spec = sourceHostSpec();

  it("offers the LLM judge only with AI review, with no AI warning in the panel", () => {
    render(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={vi.fn()} />,
    );
    const ai = screen.getByRole("switch", { name: /^AI review/ });
    const judge = screen.getByRole("switch", { name: /^LLM judge/ });
    expect(judge).toBeDisabled();
    fireEvent.click(ai);
    expect(judge).toBeEnabled();
    expect(screen.queryByText(/can make mistakes/)).not.toBeInTheDocument();
    fireEvent.click(judge);
    fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    expect(hoisted.generate).toHaveBeenLastCalledWith(
      { connectionId: "c2", aiReview: true, llmJudge: true },
      expect.anything(),
    );
  });

  it("shows the command, composed from the saved settings and the issued scan", () => {
    render(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={vi.fn()} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    expect(hoisted.generate).toHaveBeenLastCalledWith(
      { connectionId: "c2", aiReview: false, llmJudge: false },
      expect.anything(),
    );
    const opts = hoisted.generate.mock.lastCall?.[1] as MutateOptions;
    act(() => opts.onSuccess(issuedScan()));
    // The command as one line: its " \\" line continuations joined.
    const code = (screen.getByText(/AMP_SCAN_TOKEN=tok-1/).textContent ?? "").replace(/ \\\n\s*/g, " ");
    expect(code).toContain("-e AMP_URL=https://amp.example.com");
    expect(code).toContain("ghcr.io/wso2/scanner:1.0 scan --scan-id s9");
    expect(code).toContain("--github-org wso2 --all-repos");
    expect(screen.getByText(/Shown once: copy it now/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Generate" })).not.toBeInTheDocument();
  });

  it("never shows a command again after closing, and closes only with Close", () => {
    const onClose = vi.fn();
    const { rerender } = render(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={onClose} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    const opts = hoisted.generate.mock.lastCall?.[1] as MutateOptions;
    act(() => opts.onSuccess(issuedScan({ token: "secret", expiresAt: "bad date" })));
    // The command is kept only in the dialog, not in the mutation cache.
    expect(hoisted.resetGenerate).toHaveBeenCalled();
    expect(screen.getByText(/valid for 1 hour;/)).toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(1);

    rerender(
      <GenerateCommandDialog orgName="acme" target={null} spec={spec} onClose={onClose} />,
    );
    rerender(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={onClose} />,
    );
    expect(screen.queryByText(/AMP_SCAN_TOKEN=secret/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Generate" })).toBeInTheDocument();
  });

  it("says why the command could not be generated", () => {
    render(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={vi.fn()} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    const opts = hoisted.generate.mock.lastCall?.[1] as MutateOptions;
    act(() => opts.onError({ message: "Not found" }));
    expect(screen.getByText("Not found")).toBeInTheDocument();
  });
});

describe("ConnectionEditor (source code repositories)", () => {
  const githubSpec = (): ShadowProviderSpec =>
    sourceHostSpec({
      endpoints: [
        {
          key: "api",
          label: "GitHub API URL",
          default: "https://api.github.com",
          placeholders: [],
          advanced: true,
          help: "Change only for GitHub Enterprise Server.",
        },
      ],
      fields: [
        { key: "organization", label: "Organization or user account", kind: "text", required: true },
        { key: "allRepositories", label: "Scan every repository", kind: "checkbox", required: false },
        {
          key: "repositories",
          label: "Repositories",
          kind: "list",
          required: true,
          option: "!allRepositories",
        },
        {
          key: "privateRepositories",
          label: "Include private repositories",
          kind: "checkbox",
          required: false,
          defaultBool: true,
        },
        {
          key: "readCiFiles",
          label: "Also read CI pipeline definitions",
          kind: "checkbox",
          required: false,
          defaultBool: true,
        },
      ],
    });
  const openNew = () => {
    hoisted.providers = [githubSpec()];
    router(
      <Routes>
        <Route path="/new" element={<ConnectionEditor orgName="acme" listPath="/list" />} />
        <Route path="/list" element={<div>Platforms list</div>} />
      </Routes>,
      "/new",
    );
    fireEvent.click(screen.getByRole("button", { name: "Connect GitHub" }));
  };
  // The command as one line: its " \\" line continuations joined.
  const commandText = () =>
    (screen.getByText(/--scan-id/).textContent ?? "").replace(/ \\\n\s*/g, " ");

  it("shows the scan command next to the form, following the organization field", () => {
    openNew();
    expect(screen.getByText("Scan command")).toBeInTheDocument();
    expect(commandText()).toContain("--github-org <organization> --repos=<repositories>");
    expect(commandText()).toContain("-e AMP_SCAN_TOKEN=<scan-token>");
    fireEvent.change(screen.getByRole("textbox", { name: /Organization or user account/ }), {
      target: { value: "acme" },
    });
    expect(commandText()).toContain("--github-org acme --repos=<repositories>");
    fireEvent.click(screen.getByRole("checkbox", { name: "Scan every repository" }));
    expect(commandText()).toContain("--github-org acme --all-repos --include-private --read-ci");
    expect(screen.getByText(/filled in when you save/)).toBeInTheDocument();
    // The command comes with saving: there is no separate Generate command here.
    expect(screen.queryByRole("button", { name: "Generate command" })).not.toBeInTheDocument();
  });

  it("adds the platform, then issues the command and shows it filled in", () => {
    openNew();
    fireEvent.change(screen.getByRole("textbox", { name: /Organization or user account/ }), {
      target: { value: "acme" },
    });
    fireEvent.click(screen.getByRole("checkbox", { name: "Scan every repository" }));
    fireEvent.click(screen.getByRole("switch", { name: /^AI review/ }));
    fireEvent.click(screen.getByRole("button", { name: "Add platform and get command" }));
    expect(hoisted.createConnection).toHaveBeenCalledTimes(1);
    expect(hoisted.createConnection.mock.lastCall?.[0]).toMatchObject({
      provider: "github",
      settings: { organization: "acme", allRepositories: true },
    });
    expect(hoisted.generate).not.toHaveBeenCalled();
    const created = hoisted.createConnection.mock.lastCall?.[1] as {
      onSuccess: (c: { id: string; name: string }) => void;
    };
    act(() => created.onSuccess({ id: "c9", name: "GitHub connection" }));
    expect(hoisted.generate).toHaveBeenLastCalledWith(
      { connectionId: "c9", aiReview: true, llmJudge: false },
      expect.anything(),
    );
    const opts = hoisted.generate.mock.lastCall?.[1] as RunnerMutateOptions;
    act(() => opts.onSuccess(issuedScan()));
    // Kept only in the page's state, not in the mutation cache.
    expect(hoisted.resetGenerate).toHaveBeenCalled();
    expect(commandText()).toContain("-e AMP_SCAN_TOKEN=tok-1");
    expect(commandText()).toContain("ghcr.io/wso2/scanner:1.0 scan --scan-id s9");
    expect(commandText()).toContain("--ai-review");
    expect(screen.getByText(/Shown once: copy it now/)).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /Organization or user account/ })).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Add platform and get command" }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/list?tab=platforms");
  });

  it("retries only the scan command when it fails after the platform is saved", () => {
    openNew();
    fireEvent.change(screen.getByRole("textbox", { name: /Organization or user account/ }), {
      target: { value: "acme" },
    });
    fireEvent.click(screen.getByRole("checkbox", { name: "Scan every repository" }));
    fireEvent.click(screen.getByRole("button", { name: "Add platform and get command" }));
    const created = hoisted.createConnection.mock.lastCall?.[1] as {
      onSuccess: (c: { id: string; name: string }) => void;
    };
    act(() => created.onSuccess({ id: "c9", name: "GitHub connection" }));
    const opts = hoisted.generate.mock.lastCall?.[1] as RunnerMutateOptions;
    act(() => opts.onError({ message: "Scanner unavailable" }));
    expect(screen.getByText("Scanner unavailable")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(hoisted.createConnection).toHaveBeenCalledTimes(1);
    expect(hoisted.generate).toHaveBeenCalledTimes(2);
    expect(hoisted.generate).toHaveBeenLastCalledWith(
      { connectionId: "c9", aiReview: false, llmJudge: false },
      expect.anything(),
    );
  });

  it("keeps the issued command on what was saved: locked chips cannot be removed", () => {
    openNew();
    fireEvent.change(screen.getByRole("textbox", { name: /Organization or user account/ }), {
      target: { value: "acme" },
    });
    const repos = screen.getByRole("textbox", { name: /^Repositories/ });
    fireEvent.change(repos, { target: { value: "payments" } });
    fireEvent.keyDown(repos, { key: "Enter" });
    // Removable while the form is open.
    expect(screen.getByRole("button", { name: "payments" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add platform and get command" }));
    const created = hoisted.createConnection.mock.lastCall?.[1] as {
      onSuccess: (c: { id: string; name: string }) => void;
    };
    act(() => created.onSuccess({ id: "c9", name: "GitHub connection" }));
    const opts = hoisted.generate.mock.lastCall?.[1] as RunnerMutateOptions;
    act(() => opts.onSuccess(issuedScan()));
    // Locked: the chip is plain text, not a removable button, so no key removes it.
    expect(screen.queryByRole("button", { name: "payments" })).not.toBeInTheDocument();
    fireEvent.keyUp(screen.getByText("payments"), { key: "Delete" });
    expect(screen.getByText("payments")).toBeInTheDocument();
    expect(commandText()).toContain("--repos=payments");
  });
});

describe("OnboardFinding", () => {
  const renderOnboard = () =>
    router(
      <Routes>
        <Route
          path="/organizations/:orgId/shadow-agent-discovery/agents/:findingId/onboard"
          element={
            <OnboardFinding orgName="acme" listPath="/organizations/acme/shadow-agent-discovery" />
          }
        />
      </Routes>,
      "/organizations/acme/shadow-agent-discovery/agents/f1/onboard",
    );

  it("explains onboarding as an external agent, then registers and links it", () => {
    hoisted.detail = detail();
    renderOnboard();
    expect(screen.getByRole("heading", { name: "Onboard Support Bot" })).toBeInTheDocument();
    expect(screen.getByText("What onboarding does")).toBeInTheDocument();
    expect(screen.getByText(/externally-hosted agent/)).toBeInTheDocument();
    expect(screen.getByText("After you register")).toBeInTheDocument();
    expect(screen.getByText("Set up the agent")).toBeInTheDocument();
    expect(screen.queryByText(/Platform-hosted|build.*from source/i)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    expect(hoisted.createAgent).toHaveBeenCalledTimes(1);
    const [req, opts] = hoisted.createAgent.mock.calls[0];
    expect(req.params).toEqual({ orgName: "acme", projName: "default" });
    expect(req.body).toMatchObject({
      name: "support-bot",
      displayName: "Support Bot",
      provisioning: { type: "external" },
      agentType: { type: "external-agent-api" },
    });
    expect(req.body.description).toContain("Discovered by Shadow Agent Discovery");
    opts.onSuccess();
    expect(hoisted.onboard).toHaveBeenCalledWith(
      { projectName: "default", agentName: "support-bot" },
      expect.anything(),
    );
  });

  it("refuses an invalid agent name before calling anything", () => {
    hoisted.detail = detail();
    renderOnboard();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "9bad" } });
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    expect(hoisted.createAgent).not.toHaveBeenCalled();
  });
});

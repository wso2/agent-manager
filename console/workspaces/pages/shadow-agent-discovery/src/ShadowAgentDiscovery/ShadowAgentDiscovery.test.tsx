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
import { fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import type {
  ShadowAgentSummary,
  ShadowConnectionResponse,
  ShadowFindingDetail,
  ShadowFindingListQuery,
  ShadowFindingSummary,
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
  status: "new",
  ownerHint: {},
  firstSeenAt: "2026-11-01T00:00:00Z",
  lastSeenAt: "2026-11-10T00:00:00Z",
  present: true,
  foundBy: [{ findingId: "f1", connectionId: "c1", connectionName: "OpenAI prod", present: true }],
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
  update: vi.fn(),
  start: vi.fn(),
  clear: vi.fn(),
  deleteAgent: vi.fn(),
  deleteAll: vi.fn(),
  test: vi.fn(),
  addConfirmation: vi.fn(),
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
  useUpdateShadowFinding: () => ({ mutate: hoisted.update, isPending: false }),
  useDeleteShadowFinding: () => ({ mutate: hoisted.deleteAgent, isPending: false }),
  useDeleteShadowFindings: () => ({ mutate: hoisted.deleteAll, isPending: false }),
  useTokenScopes: () => ({ enforced: hoisted.enforced, resolved: true, scopes: hoisted.scopes }),
  useShadowSummary: () => ({ data: hoisted.summary, isLoading: false, error: null }),
  useShadowProviders: () => ({ data: undefined }),
  useShadowConnections: () => ({
    data: { connections: hoisted.connections },
    isLoading: false,
    error: null,
  }),
  useShadowScans: () => ({ data: { scans: hoisted.scans }, isLoading: false, error: null }),
  useShadowScan: () => ({ data: hoisted.scans[0], isLoading: false, error: null }),
  useClearShadowScans: () => ({ mutate: hoisted.clear, isPending: false }),
  useStartShadowScan: () => ({ mutate: hoisted.start, isPending: false }),
  useDeleteShadowConnection: () => ({ mutate: vi.fn(), isPending: false }),
  useTestShadowConnection: () => ({ mutate: hoisted.test, isPending: false }),
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
    useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
    useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
  };
});

import { FindingsTab } from "./subComponents/FindingsTab";
import { FindingDetail, relationLabel } from "./subComponents/FindingDetail";
import { OverviewPanel } from "./subComponents/OverviewPanel";
import { RunScanButton } from "./subComponents/RunScanButton";
import { ConnectionsTab, ScopedKeysChip } from "./subComponents/ConnectionsTab";
import { ScansTab } from "./subComponents/ScansTab";
import { ShadowAgentDiscovery } from "./ShadowAgentDiscovery";
import { SourceRepoPreview } from "../sourceRepoScan/SourceRepoPreview";
import { AnalyzeSection } from "./subComponents/AnalyzeSection";

function ShowLocation() {
  const l = useLocation();
  return <div data-testid="location">{`${l.pathname}${l.search}`}</div>;
}

const router = (ui: React.ReactNode, path = "/org/acme/shadow-agent-discovery") =>
  render(
    <MemoryRouter
      initialEntries={[path]}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      {ui}
      <ShowLocation />
    </MemoryRouter>,
  );

const page = (path = "/org/acme/shadow-agent-discovery") =>
  router(
    <Routes>
      <Route path="/org/:orgId/shadow-agent-discovery/*" element={<ShadowAgentDiscovery />} />
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
    page("/org/acme/shadow-agent-discovery?tab=agents&platform=bedrock");
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
    hoisted.summary = { totalFindings: 0, byProvider: {}, byStatus: {}, connections: 0 };
    router(<OverviewPanel {...props} />);
    expect(screen.getByText("Add a platform to start")).toBeInTheDocument();
    for (const step of [
      "Add a platform",
      "Test the connection",
      "Run a scan",
      "Review the unregistered agents",
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
      byStatus: { new: 4, reviewed: 1 },
      connections: 2,
      latestScan: scan({ connectionIds: ["c1", "c2"] }),
    };
    router(<OverviewPanel {...props} />);
    expect(screen.getByText("Unregistered agents")).toBeInTheDocument();
    expect(screen.getByText("To review")).toBeInTheDocument();
    expect(screen.getByText("Platforms connected")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText(/2 platforms scanned/)).toBeInTheDocument();
    expect(screen.queryByText(/score/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/verdict/i)).not.toBeInTheDocument();
    fireEvent.click(screen.getByText("Amazon Bedrock · 2"));
    expect(props.onOpenAgents).toHaveBeenCalledWith("bedrock");
    fireEvent.click(screen.getByRole("button", { name: "All scans" }));
    expect(props.onOpenScans).toHaveBeenCalled();
    fireEvent.click(screen.getAllByRole("button", { name: "View agents" })[0]);
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/org/acme/shadow-agent-discovery/scans/s1",
    );
  });
});

describe("FindingsTab", () => {
  it("lists each agent with its platform, who found it, state and status", () => {
    hoisted.findings = [
      finding(),
      finding({
        id: "f2",
        displayName: "triage",
        provider: "bedrock",
        kind: "runtime",
        scope: "us-east-1",
        state: "READY",
        status: "reviewed",
        foundBy: [
          { findingId: "f2", connectionId: "c2", connectionName: "AWS prod", present: true },
          { findingId: "f3", connectionId: "c3", connectionName: "AWS audit", present: true },
        ],
      }),
    ];
    router(<FindingsTab orgName="acme" />);
    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent);
    expect(headers).toEqual(["Agent", "Platform", "Found by", "State", "Last seen", "Status"]);
    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    expect(screen.getByText("AgentCore runtime · us-east-1")).toBeInTheDocument();
    expect(screen.getByText("Amazon Bedrock")).toBeInTheDocument();
    expect(screen.getByText("AWS prod, AWS audit")).toBeInTheDocument();
    expect(screen.getByText("READY")).toBeInTheDocument();
    expect(screen.getByText("Reviewed")).toBeInTheDocument();
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
      "Found by",
      "State",
      "Last seen",
      "Status",
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
      "/org/acme/shadow-agent-discovery/agents/f1",
    );
  });

  it("filters by the five agent platforms and by review status", () => {
    hoisted.findings = [finding()];
    router(<FindingsTab orgName="acme" />);
    expect(openSelect("Platform")).toEqual([
      "All platforms",
      "OpenAI",
      "Anthropic",
      "Amazon Bedrock",
      "Google Agent Platform",
      "Microsoft Foundry",
    ]);
    fireEvent.click(screen.getByRole("option", { name: "Microsoft Foundry" }));
    expect(lastQuery()).toMatchObject({ provider: "foundry", offset: 0 });
    expect(openSelect("Status")).toEqual(["All statuses", "New", "Reviewed", "Dismissed"]);
    fireEvent.click(screen.getByRole("option", { name: "Dismissed" }));
    expect(lastQuery()).toMatchObject({ provider: "foundry", status: "dismissed" });
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
          path="/org/:orgId/shadow-agent-discovery/agents/:findingId"
          element={
            <FindingDetail
              orgName="acme"
              canManage={canManage}
              listPath="/org/acme/shadow-agent-discovery"
            />
          }
        />
      </Routes>,
      "/org/acme/shadow-agent-discovery/agents/f1",
    );

  it("shows the evidence, the listed details and who found the agent, without scores", () => {
    hoisted.detail = detail();
    renderDetail();
    expect(screen.getByRole("heading", { name: "Support Bot" })).toBeInTheDocument();
    expect(screen.getByText("OpenAI · Agent")).toBeInTheDocument();
    expect(screen.getByText("Agent object")).toBeInTheDocument();
    expect(screen.getByText(/an item in data/)).toBeInTheDocument();
    expect(screen.getByText("toolCount=2")).toBeInTheDocument();
    expect(screen.getByText("Model")).toBeInTheDocument();
    expect(screen.getByText("gpt-4.1")).toBeInTheDocument();
    expect(screen.getByText("agent_abc")).toBeInTheDocument();
    expect(screen.getByText("Found by")).toBeInTheDocument();
    expect(screen.queryByText("Owner")).not.toBeInTheDocument();
    expect(screen.queryByText(/score/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/verdict/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/onboard/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/no longer listed/i)).not.toBeInTheDocument();
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
    expect(screen.getByText("Owner")).toBeInTheDocument();
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
      "/org/acme/shadow-agent-discovery/agents/f7",
    );
    expect(relationLabel("possible_duplicate")).toBe("Possible duplicate");
  });

  it("reviews, dismisses and reopens the agent", () => {
    hoisted.detail = detail();
    const { unmount } = renderDetail();
    fireEvent.click(screen.getByRole("button", { name: "Mark reviewed" }));
    expect(hoisted.update).toHaveBeenLastCalledWith({ status: "reviewed" });
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(hoisted.update).toHaveBeenLastCalledWith({ status: "dismissed" });
    unmount();
    hoisted.detail = detail({ status: "dismissed" });
    renderDetail();
    fireEvent.click(screen.getByRole("button", { name: "Reopen" }));
    expect(hoisted.update).toHaveBeenLastCalledWith({ status: "new" });
  });

  it("has no review buttons for a read-only user", () => {
    hoisted.detail = detail();
    renderDetail(false);
    expect(screen.queryByRole("button", { name: "Mark reviewed" })).not.toBeInTheDocument();
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
      byStatus: {},
      connections: 2,
      latestScan: scan({ status: "running" }),
    };
    const { unmount } = router(<RunScanButton orgName="acme" />);
    expect(screen.getByRole("button", { name: "Scan running…" })).toBeDisabled();
    unmount();
    hoisted.summary = undefined;
    hoisted.connections = [];
    router(<RunScanButton orgName="acme" />);
    expect(screen.getByRole("button", { name: "Run scan" })).toBeDisabled();
  });
});

describe("ConnectionsTab (Platforms)", () => {
  const renderTab = () =>
    router(<ConnectionsTab orgName="acme" canManage onAdd={vi.fn()} onEdit={vi.fn()} />);

  it("lists platforms with a Scan action for agent platforms only", () => {
    hoisted.connections = [
      connection(),
      connection({ id: "c2", name: "Repos", provider: "github" }),
    ];
    renderTab();
    expect(screen.getByRole("button", { name: "Add platform" })).toBeInTheDocument();
    const headers = screen.getAllByRole("columnheader").map((h) => h.textContent);
    expect(headers.slice(0, 2)).toEqual(["Name", "Platform"]);
    fireEvent.click(screen.getByRole("button", { name: "Scan OpenAI prod" }));
    expect(hoisted.start).toHaveBeenCalledWith({ connectionIds: ["c1"] });
    expect(screen.queryByRole("button", { name: "Scan Repos" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Test Repos" })).toBeInTheDocument();
  });

  it("disables Scan while a scan runs", () => {
    hoisted.connections = [connection()];
    hoisted.summary = {
      totalFindings: 0,
      byProvider: {},
      byStatus: {},
      connections: 1,
      latestScan: scan({ status: "queued" }),
    };
    renderTab();
    expect(screen.getByRole("button", { name: "Scan OpenAI prod" })).toBeDisabled();
  });

  it("deletes a platform after saying what is removed", () => {
    hoisted.connections = [connection()];
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Delete OpenAI prod" }));
    expect(hoisted.addConfirmation).toHaveBeenCalledWith(
      expect.objectContaining({
        description: expect.stringContaining("every agent this platform found"),
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
      "/org/acme/shadow-agent-discovery/scans/s1",
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
});

describe("SourceRepoPreview", () => {
  it("labels the sample data and lists suspects of the scanned hosts", () => {
    router(<SourceRepoPreview hosts={[{ connectionName: "gh", provider: "github" }]} />);
    expect(screen.getByText("Sample data")).toBeInTheDocument();
    expect(screen.getByText("acme-corp/support-assistant")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: /Show hits for acme-corp\/support-assistant/ }),
    );
    expect(screen.getByText(/value not stored/)).toBeInTheDocument();
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

  it("says what an agent platform's scan lists, with what is missing", () => {
    render(<AnalyzeSection providerName="Amazon Bedrock" statuses={statuses} />);
    expect(screen.getByText("What does Agent Manager list?")).toBeInTheDocument();
    expect(
      screen.getByText("1 of 2 available with these credentials and options."),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "See what it lists" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("What Agent Manager lists");
    expect(dialog).toHaveTextContent("only the platform's agent listings are called");
    expect(dialog).toHaveTextContent("Available (1)");
    expect(dialog).toHaveTextContent("Not available (1)");
    expect(dialog).toHaveTextContent("Needs: Scan the AWS Agent Registry");
  });

  it("keeps the analysis wording for a source repository host", () => {
    render(<AnalyzeSection providerName="GitHub" statuses={statuses} preview />);
    expect(screen.getByText("What can Agent Manager analyze?")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "See what it can analyze" }));
    expect(screen.getByRole("dialog")).toHaveTextContent(
      "Source code analysis is not available yet",
    );
  });

  it("renders nothing for a platform without capabilities", () => {
    const { container } = render(<AnalyzeSection providerName="X" statuses={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});

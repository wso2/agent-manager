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
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import type {
  ShadowAgentSummary,
  ShadowConnectionResponse,
  ShadowFindingDetail,
  ShadowFindingListQuery,
  ShadowFindingSummary,
  ShadowProvider,
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
  hidden: false,
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

/**
 * A scan of OpenAI prod (done, 2 agents), AWS prod (being scanned) and Inventory (not
 * started). Its findingsCount is the sum of the finished connections'.
 */
const runningScan = (over: Partial<ShadowScanResponse> = {}): ShadowScanResponse =>
  scan({
    status: "running",
    connectionIds: ["c1", "c2", "c3"],
    startedAt: "2026-11-10T00:00:00Z",
    findingsCount: 2,
    results: [
      {
        connectionId: "c1",
        connectionName: "OpenAI prod",
        provider: "openai",
        outcome: "ok",
        findingsCount: 2,
        calls: [],
      },
      {
        connectionId: "c2",
        connectionName: "AWS prod",
        provider: "bedrock",
        outcome: "running",
        findingsCount: 0,
        startedAt: "2026-11-10T00:01:00Z",
        calls: [
          { call: "ListAgents", outcome: "ok", items: 2 },
          { call: "ListKnowledgeBases", outcome: "permission_denied", items: 0 },
          { call: "ListAgentRuntimes", outcome: "ok", items: 5 },
        ],
      },
      {
        connectionId: "c3",
        connectionName: "",
        provider: "",
        outcome: "queued",
        findingsCount: 0,
        calls: [],
      },
    ],
    ...over,
  });

/**
 * runningScan once cancelled: AWS prod and Inventory did not finish. As the API
 * reports it, the scan's findingsCount is the sum of the results kept.
 */
const cancelledScan = (): ShadowScanResponse => {
  const results = runningScan().results.map((x) =>
    x.outcome === "ok"
      ? x
      : { ...x, outcome: "failed", message: "cancelled", findingsCount: 0 },
  );
  return {
    ...runningScan(),
    status: "failed",
    results,
    findingsCount: results.reduce((n, x) => n + x.findingsCount, 0),
  };
};

/** Bring your own inventory, as the provider catalog gives it. */
const inventorySpec = (): ShadowProviderSpec => ({
  provider: "custom",
  displayName: "Bring your own inventory",
  description: "Agents from a file you upload.",
  endpoints: [],
  fields: [],
  permissions: [],
  docs: [],
  capabilities: [],
  fileUpload: true,
  setupSteps: ["Download a template and list your agents in it.", "Upload the file."],
  inventoryFields: [
    { name: "id", required: true, description: "Your ID of the agent.", example: "hr-bot" },
    { name: "name", required: true, description: "The agent's name.", example: "HR, \"helper\"" },
    { name: "owner", required: false, description: "Who owns it.", example: "Ana" },
    {
      name: "labels",
      required: false,
      description: "Labels.",
      example: "team=support; region=eu",
    },
  ],
});

/** What an upload of 12 agents with one row left out returns. */
const uploadResult = () => ({
  scan: { ...scan({ id: "s-up", connectionIds: ["c3"] }) },
  agentsRead: 13,
  agentsSaved: 12,
  leftOut: 1,
  problems: [{ row: 4, message: "no name", leftOut: true }],
  problemCount: 1,
  ignoredColumns: ["secret"],
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
  connection: undefined as ShadowConnectionResponse | undefined,
  connectionError: null as unknown,
  projectsError: null as unknown,
  scans: [] as ShadowScanResponse[],
  scanDetail: null as ShadowScanResponse | null,
  scanIds: [] as (string | undefined)[],
  scanLoading: false,
  scanError: null as unknown,
  createAgent: vi.fn(),
  onboard: vi.fn(),
  setHidden: vi.fn(),
  start: vi.fn(),
  clear: vi.fn(),
  cancel: vi.fn(),
  refreshFindings: vi.fn(),
  upload: vi.fn(),
  resetUpload: vi.fn(),
  deleteAgent: vi.fn(),
  deleteAll: vi.fn(),
  test: vi.fn(),
  generate: vi.fn(),
  resetGenerate: vi.fn(),
  addConfirmation: vi.fn(),
  providers: [] as ShadowProviderSpec[],
  createConnection: vi.fn(),
  updateConnection: vi.fn(),
  uploadInventory: vi.fn(),
  resetInventory: vi.fn(),
  downloadTemplate: vi.fn(),
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
  useSetShadowFindingHidden: () => ({ mutate: hoisted.setHidden, isPending: false }),
  useCreateAgent: () => ({ mutate: hoisted.createAgent, isPending: false }),
  useListProjects: () => ({
    data: hoisted.projectsError
      ? undefined
      : { projects: [{ name: "default", displayName: "Default" }] },
    isLoading: false,
    error: hoisted.projectsError,
  }),
  useDeleteShadowFinding: () => ({ mutate: hoisted.deleteAgent, isPending: false }),
  useDeleteShadowFindings: () => ({ mutate: hoisted.deleteAll, isPending: false }),
  useTokenScopes: () => ({ enforced: hoisted.enforced, resolved: true, scopes: hoisted.scopes }),
  useShadowSummary: () => ({ data: hoisted.summary, isLoading: false, error: null }),
  useShadowProviders: () => ({
    data: hoisted.providers.length > 0 ? { providers: hoisted.providers, signals: [] } : undefined,
    isLoading: false,
  }),
  useShadowConnection: () => ({
    data: hoisted.connection,
    isLoading: false,
    error: hoisted.connectionError,
  }),
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
  useCancelShadowScan: () => ({ mutate: hoisted.cancel, isPending: false }),
  useRefreshShadowFindings: () => hoisted.refreshFindings,
  useUploadShadowScanResults: () => ({
    mutate: hoisted.upload,
    reset: hoisted.resetUpload,
    isPending: false,
  }),
  isRequestTooLargeError: () => false,
  useUploadShadowInventory: () => ({
    mutate: hoisted.uploadInventory,
    reset: hoisted.resetInventory,
    isPending: false,
  }),
  useDownloadShadowInventoryTemplate: () => ({
    mutate: hoisted.downloadTemplate,
    isPending: false,
  }),
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
import { OverviewPanel } from "./subComponents/OverviewPanel";
import { RunScanButton } from "./subComponents/RunScanButton";
import { ConnectionsTab, ScopedKeysChip } from "./subComponents/ConnectionsTab";
import { ScansTab } from "./subComponents/ScansTab";
import { ScanProgressCard } from "./subComponents/ScanProgress";
import { ScanFindings } from "./subComponents/ScanFindings";
import { ShadowAgentDiscovery } from "./ShadowAgentDiscovery";
import { AnalyzeSection } from "./subComponents/AnalyzeSection";
import { latestCloudScan, ScanStatusChip } from "./shadowUi";
import { SourceScanGuide } from "./subComponents/SourceScanGuide";
import { DocLinks } from "./subComponents/PermissionsPanel";
import { GenerateCommandDialog } from "./subComponents/GenerateCommandDialog";
import { ConnectionEditor } from "./subComponents/ConnectionEditor";
import {
  csvValue,
  exampleCsv,
  exampleJson,
  inventoryFormatOf,
  inventoryUploadErrorMessage,
} from "./subComponents/InventoryFilePanel";

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
  hoisted.connection = undefined;
  hoisted.connectionError = null;
  hoisted.projectsError = null;
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

  it("has the Overview, Scans and Platforms tabs: the unmanaged agents are in the registry", () => {
    page();
    expect(screen.getByRole("heading", { name: "Shadow Agent Discovery" })).toBeInTheDocument();
    const tabs = screen.getAllByRole("tab").map((t) => t.textContent);
    expect(tabs).toEqual(["Overview", "Sources", "Scans"]);
    expect(screen.queryByRole("tab", { name: "Connections" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Findings" })).not.toBeInTheDocument();
    // Run scan is with the AI platforms on the Sources tab, not on the whole page.
    expect(screen.queryByRole("button", { name: "Run scan" })).not.toBeInTheDocument();
  });

  it("opens the Overview for an old link to the Unmanaged Agents tab", () => {
    page("/organizations/acme/shadow-agent-discovery?tab=agents&platform=bedrock");
    expect(screen.getByRole("tab", { name: "Overview" })).toHaveAttribute("aria-selected", "true");
  });

  it("switches tabs through the URL", () => {
    page();
    fireEvent.click(screen.getByRole("tab", { name: "Sources" }));
    expect(screen.getByTestId("location")).toHaveTextContent("?tab=platforms");
    fireEvent.click(screen.getByRole("tab", { name: "Scans" }));
    expect(screen.getByTestId("location")).toHaveTextContent("?tab=scans");
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
    expect(screen.getByText("Add a source to start")).toBeInTheDocument();
    for (const step of [
      "Add a source",
      "Test the connector",
      "Run a scan",
      "Onboard the unmanaged agents",
    ]) {
      expect(screen.getAllByText(step).length).toBeGreaterThan(0);
    }
    fireEvent.click(screen.getByRole("button", { name: "Add source" }));
    expect(props.onConnect).toHaveBeenCalled();
  });

  it("shows the steps until the first scan, with no platform rows", () => {
    hoisted.summary = { totalFindings: 0, byProvider: {}, onboarded: 0, connections: 1 };
    router(<OverviewPanel {...props} />);
    expect(screen.getByText("No scan yet.")).toBeInTheDocument();
    expect(screen.getByText("Get started")).toBeInTheDocument();
    expect(screen.queryByText("Agents by source")).not.toBeInTheDocument();
  });

  it("finishes Get started when the only agents came from an uploaded file", () => {
    // An upload is not a scan: the summary has agents but no latest scan.
    hoisted.summary = { totalFindings: 4, byProvider: { custom: 4 }, onboarded: 0, connections: 1 };
    router(<OverviewPanel {...props} />);
    expect(screen.queryByText("No scan yet.")).not.toBeInTheDocument();
    expect(
      screen.getByText(
        "No platform scan yet. The agents below include those from uploaded inventory files.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText("Get started")).not.toBeInTheDocument();
    expect(screen.getByText("Agents by source")).toBeInTheDocument();
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
    expect(screen.getByText("Unmanaged agents")).toBeInTheDocument();
    expect(screen.getByText("Managed")).toBeInTheDocument();
    expect(screen.getByText("Connectors")).toBeInTheDocument();
    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText("1")).toBeInTheDocument();
    expect(screen.getByText(/Latest scan .* 2 connectors/)).toBeInTheDocument();
    expect(screen.queryByText(/score/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/verdict/i)).not.toBeInTheDocument();
    expect(screen.queryByText("Get started")).not.toBeInTheDocument();
    const rows = screen.getAllByRole("button", { name: /\d+ agents?$/ });
    expect(rows.map((r) => r.getAttribute("aria-label"))).toEqual([
      "OpenAI, 3 agents",
      "Amazon Bedrock, 2 agents",
    ]);
    fireEvent.click(screen.getByRole("button", { name: "Amazon Bedrock, 2 agents" }));
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
  it("lists each agent with its platform, source and onboarding status", () => {
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
    expect(headers).toEqual(["Agent", "Source", "Connector", "Last seen", "Status"]);
    expect(lastQuery()).toMatchObject({ onboarded: "false" });
    expect(screen.getByText("Support Bot")).toBeInTheDocument();
    // The second line is the type only; the scope, state and IDs are on the agent's page.
    expect(screen.getByText("AgentCore runtime")).toBeInTheDocument();
    expect(screen.queryByText(/us-east-1/)).not.toBeInTheDocument();
    expect(screen.queryByText("READY")).not.toBeInTheDocument();
    expect(screen.getByText("Amazon Bedrock")).toBeInTheDocument();
    expect(screen.getByText("AWS prod, AWS audit")).toBeInTheDocument();
    expect(screen.getByText("Managed")).toBeInTheDocument();
    // The default filter and the support bot's chip.
    expect(screen.getAllByText("Unmanaged")).toHaveLength(2);
    expect(screen.queryByText(/Reviewed|Dismissed/)).not.toBeInTheDocument();
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

  it("marks code agents confirmed by AI-enhanced detection, not the ones the rules proved", async () => {
    const code = (
      id: string,
      displayName: string,
      reviewedBy: ShadowFindingSummary["reviewedBy"],
    ) => finding({ id, displayName, provider: "github", kind: "code_agent", externalId: id, reviewedBy });
    hoisted.findings = [
      code("a", "triage", "ai_review"),
      code("b", "planner", "ai_review_judge"),
      code("c", "router", "rules"),
    ];
    router(<FindingsTab orgName="acme" />);
    const row = (name: string) => screen.getByText(name).closest("tr") as HTMLElement;
    expect(within(row("triage")).getByText("AI")).toBeInTheDocument();
    expect(within(row("planner")).getByText("AI")).toBeInTheDocument();
    expect(within(row("router")).queryByText("AI")).not.toBeInTheDocument();
    expect(screen.queryByText(/can make mistakes/)).not.toBeInTheDocument();
    fireEvent.mouseOver(within(row("triage")).getByText("AI"));
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Confirmed by AI-enhanced detection, not by the scan's rules. " +
        "Open the agent to check its evidence.",
    );
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
    // The delete column has no title; each row keeps its delete button.
    expect(headers).toEqual(["Agent", "Source", "Connector", "Last seen", "Status", ""]);
    fireEvent.click(screen.getByRole("button", { name: "Delete Support Bot" }));
    expect(screen.getByTestId("location")).toHaveTextContent("/");
    expect(screen.getByTestId("location")).not.toHaveTextContent("/agents/");
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.title).toBe("Delete Support Bot?");
    expect(opts.description).toContain("for every connector that found it");
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
    expect(opts.title).toBe("Delete all unmanaged agents?");
    expect(opts.description).toContain("for every connector, whatever the filters show");
    expect(opts.description).toContain("including the agents their platforms no longer list");
    expect(opts.description).toContain("Connectors and scan history are kept");
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

  it("marks an agent its platform no longer lists, and opens an agent in the registry", () => {
    hoisted.findings = [finding({ present: false })];
    router(<FindingsTab orgName="acme" scanId="s1" />);
    expect(lastQuery()).toMatchObject({ scanId: "s1", onboarded: undefined });
    expect(screen.getByText("No longer listed")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Support Bot"));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/agent-registry/unmanaged/f1",
    );
  });

  it("filters by the agent platforms, the source hosts and onboarding", () => {
    hoisted.findings = [finding()];
    router(<FindingsTab orgName="acme" />);
    expect(openSelect("Source")).toEqual([
      "All sources",
      "OpenAI",
      "Anthropic",
      "Amazon Bedrock",
      "Google Agent Platform",
      "Microsoft Foundry",
      "Your inventory",
      "GitHub",
    ]);
    fireEvent.click(screen.getByRole("option", { name: "Microsoft Foundry" }));
    expect(lastQuery()).toMatchObject({ provider: "foundry", offset: 0, onboarded: "false" });
    expect(openSelect("Management")).toEqual(["All agents", "Unmanaged", "Managed"]);
    fireEvent.click(screen.getByRole("option", { name: "All agents" }));
    expect(lastQuery()).toMatchObject({ provider: "foundry", onboarded: undefined });
    openSelect("Management");
    fireEvent.click(screen.getByRole("option", { name: "Managed" }));
    expect(lastQuery()).toMatchObject({ provider: "foundry", onboarded: "true" });
  });

  it("shows the empty state, not an error, when there are no agents", () => {
    router(<FindingsTab orgName="acme" />);
    expect(screen.getByText("No unmanaged agents yet")).toBeInTheDocument();
  });

  it("shows only the error when loading fails", () => {
    hoisted.findingsError = new Error("boom");
    router(<FindingsTab orgName="acme" />);
    expect(screen.getByText(/Couldn't load the agents/)).toBeInTheDocument();
    expect(screen.queryByText("No unmanaged agents yet")).not.toBeInTheDocument();
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
    expect(screen.getByLabelText("All connectors (2)")).toBeChecked();
    fireEvent.click(screen.getByRole("button", { name: "Start scan" }));
    expect(hoisted.start).toHaveBeenCalledWith({}, expect.anything());
  });

  it("scans only the platforms chosen", () => {
    router(<RunScanButton orgName="acme" />);
    fireEvent.click(screen.getByRole("button", { name: "Run scan" }));
    fireEvent.click(screen.getByLabelText("Choose connectors"));
    expect(screen.getByRole("button", { name: "Start scan" })).toBeDisabled();
    fireEvent.click(screen.getByLabelText("AWS prod · Amazon Bedrock"));
    fireEvent.click(screen.getByRole("button", { name: "Start scan" }));
    expect(hoisted.start).toHaveBeenCalledWith({ connectionIds: ["c2"] }, expect.anything());
  });

  it("counts only the connectors a scan calls, not the source code repositories", () => {
    hoisted.connections = [
      ...hoisted.connections,
      connection({ id: "c3", name: "Repos", provider: "github" }),
    ];
    router(<RunScanButton orgName="acme" />);
    fireEvent.click(screen.getByRole("button", { name: "Run scan" }));
    expect(screen.getByLabelText("All connectors (2)")).toBeChecked();
    expect(screen.getByRole("dialog")).toHaveTextContent(
      "Source code repositories are not included",
    );
    fireEvent.click(screen.getByLabelText("Choose connectors"));
    expect(screen.queryByLabelText(/^Repos/)).not.toBeInTheDocument();
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
    const { unmount: unmountRepos } = router(<RunScanButton orgName="acme" />);
    expect(screen.getByRole("button", { name: "Run scan" })).toBeDisabled();
    unmountRepos();
    // Inventory agents come from uploaded files, not from Run scan.
    hoisted.connections = [connection({ id: "c3", name: "Inventory", provider: "custom" })];
    router(<RunScanButton orgName="acme" />);
    expect(screen.getByRole("button", { name: "Run scan" })).toBeDisabled();
  });

  it("does not count or offer Bring your own inventory sources", () => {
    hoisted.connections = [
      ...hoisted.connections,
      connection({ id: "c3", name: "Inventory", provider: "custom" }),
    ];
    router(<RunScanButton orgName="acme" />);
    fireEvent.click(screen.getByRole("button", { name: "Run scan" }));
    expect(screen.getByLabelText("All connectors (2)")).toBeChecked();
    expect(screen.getByRole("dialog")).toHaveTextContent(
      "Bring your own inventory is not included",
    );
    fireEvent.click(screen.getByLabelText("Choose connectors"));
    expect(screen.queryByLabelText(/^Inventory/)).not.toBeInTheDocument();
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
    expect(screen.getByRole("button", { name: "Add source" })).toBeInTheDocument();
    expect(screen.getByText("AI platforms")).toBeInTheDocument();
    expect(screen.getByText("Source code repositories")).toBeInTheDocument();
    const [ai, source] = screen.getAllByRole("table");
    expect(within(ai).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Name",
      "Source",
      "Last test",
      "Created",
      "",
    ]);
    expect(within(source).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Name",
      "Source",
      "What it scans",
      "Created",
      "",
    ]);
    expect(within(source).getByText("acme · All repositories · Private and public")).toBeInTheDocument();
    // Run scan is in the AI platforms section only.
    const runScan = screen.getByRole("button", { name: "Run scan" });
    expect(runScan.compareDocumentPosition(ai) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Run scan" })).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Scan OpenAI prod" }));
    expect(hoisted.start).toHaveBeenCalledWith({ connectionIds: ["c1"] }, expect.anything());
    // A source host has no credential to test and no cloud scan.
    expect(screen.queryByRole("button", { name: "Scan Repos" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Test Repos" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Edit Repos" })).toBeInTheDocument();
  });

  it("lists Bring your own inventory sources in their own section, with Upload file", async () => {
    hoisted.connections = [
      connection(),
      connection({
        id: "c3",
        name: "Inventory",
        provider: "custom",
        lastUploadAt: "2026-11-12T00:00:00Z",
      }),
      // No file uploaded yet.
      connection({ id: "c4", name: "HR inventory", provider: "custom" }),
      connection({ id: "c2", name: "Repos", provider: "github", settings: { organization: "acme" } }),
    ];
    hoisted.providers = [inventorySpec()];
    renderTab();
    const headings = screen
      .getAllByText(/^(AI platforms|Bring your own inventory|Source code repositories)$/)
      .map((h) => h.textContent);
    expect(headings).toEqual(["AI platforms", "Source code repositories", "Bring your own inventory"]);
    const [ai, , custom] = screen.getAllByRole("table");
    expect(within(ai).queryByText("Inventory")).not.toBeInTheDocument();
    expect(within(custom).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Name",
      "Source",
      "Agents updated",
      "Created",
      "",
    ]);
    const [, row, noFile] = within(custom).getAllByRole("row");
    expect(within(row).getByText("Your inventory")).toBeInTheDocument();
    // Agents updated and Created (the mocked relative time).
    expect(within(row).getAllByText("2 days ago")).toHaveLength(2);
    expect(within(noFile).getByText("No file uploaded yet")).toBeInTheDocument();
    expect(within(noFile).getAllByText("2 days ago")).toHaveLength(1);
    // Nothing to scan or test: the agents come from uploaded files.
    expect(screen.queryByRole("button", { name: "Scan Inventory" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Test Inventory" })).not.toBeInTheDocument();
    expect(within(custom).getByRole("button", { name: "Edit Inventory" })).toBeInTheDocument();
    fireEvent.click(within(custom).getByRole("button", { name: "Upload a file to Inventory" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("Upload file: Inventory");
    const upload = within(dialog).getByRole("button", { name: "Upload" });
    expect(upload).toBeDisabled();
    const file = new File(["{}"], "agents.json", { type: "application/json" });
    fireEvent.change(within(dialog).getByTestId("inventory-file-input"), {
      target: { files: [file] },
    });
    expect(dialog).toHaveTextContent("agents.json");
    fireEvent.click(upload);
    expect(hoisted.uploadInventory).toHaveBeenCalledWith(
      { connectionId: "c3", file },
      expect.anything(),
    );
    act(() => hoisted.uploadInventory.mock.calls[0][1].onSuccess(uploadResult()));
    expect(within(dialog).getByRole("alert")).toHaveTextContent("12 agents saved");
    fireEvent.click(within(dialog).getByRole("button", { name: "Done" }));
    // The title keeps the source's name while the dialog closes.
    expect(dialog).toHaveTextContent("Upload file: Inventory");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("puts Run scan with the AI platforms only, never with inventories or repositories", () => {
    hoisted.connections = [
      connection({ id: "c2", name: "Repos", provider: "github" }),
      connection({ id: "c3", name: "Inventory", provider: "custom" }),
    ];
    renderTab();
    expect(screen.queryByRole("button", { name: "Run scan" })).not.toBeInTheDocument();
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

  it("says which connector a running scan covers, marks its row and links to the scan", () => {
    hoisted.connections = [connection(), connection({ id: "c2", name: "AWS prod", provider: "bedrock" })];
    hoisted.summary = {
      totalFindings: 0,
      byProvider: {},
      onboarded: 0,
      connections: 2,
      latestScan: scan({ status: "running", results: [] }),
    };
    renderTab();
    const notice = screen.getByRole("alert");
    expect(notice).toHaveTextContent(/^Scanning OpenAI prod, started/);
    expect(screen.getAllByText("Scanning")).toHaveLength(1);
    fireEvent.click(within(notice).getByRole("button", { name: "View scan" }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/shadow-agent-discovery/scans/s1",
    );
  });

  it("says a scan is starting while the request is on its way, and marks the row", () => {
    hoisted.connections = [connection()];
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Scan OpenAI prod" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Starting a scan of OpenAI prod…");
    expect(screen.getByText("Scanning")).toBeInTheDocument();
  });

  it("shows the scan it just started at once, before the summary catches up", () => {
    hoisted.connections = [connection()];
    hoisted.summary = {
      totalFindings: 0,
      byProvider: {},
      onboarded: 0,
      connections: 1,
      latestScan: scan({ id: "s0" }),
    };
    hoisted.start.mockImplementationOnce(
      (_body: unknown, opts: { onSuccess: (s: ShadowScanResponse) => void }) =>
        opts.onSuccess(scan({ id: "s9", status: "queued", results: [], findingsCount: 0 })),
    );
    renderTab();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Scan OpenAI prod" }));
    expect(screen.getByRole("alert")).toHaveTextContent(/^Scanning OpenAI prod, started/);
    expect(screen.getByRole("button", { name: "Scan OpenAI prod" })).toBeDisabled();
  });

  it("says how a scan it saw running ended, until dismissed, and nothing for older scans", () => {
    hoisted.connections = [connection()];
    const summary = (status: ShadowScanResponse["status"]) => ({
      totalFindings: 3,
      byProvider: {},
      onboarded: 0,
      connections: 1,
      latestScan: scan({ status }),
    });
    hoisted.summary = summary("succeeded");
    const ui = () => (
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <ConnectionsTab orgName="acme" canManage onAdd={vi.fn()} onEdit={vi.fn()} />
      </MemoryRouter>
    );
    const { rerender } = render(ui());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    hoisted.summary = summary("running");
    rerender(ui());
    hoisted.summary = summary("succeeded");
    rerender(ui());
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Scan of OpenAI prod finished: 3 agents found.",
    );
    expect(screen.queryByText("Scanning")).not.toBeInTheDocument();
    fireEvent.click(within(screen.getByRole("alert")).getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
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
        description: expect.stringContaining("every agent this connector found"),
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Delete Repos" }));
    expect(hoisted.addConfirmation).toHaveBeenLastCalledWith(
      expect.objectContaining({
        description: expect.stringContaining("Every agent found in these repositories"),
      }),
    );
  });

  it("shows how far a running scan got: the connectors done and the last step", () => {
    hoisted.connections = [
      connection(),
      connection({ id: "c2", name: "AWS prod", provider: "bedrock" }),
      connection({ id: "c3", name: "Google prod", provider: "google" }),
    ];
    hoisted.summary = {
      totalFindings: 2,
      byProvider: {},
      onboarded: 0,
      connections: 3,
      latestScan: runningScan(),
    };
    const { unmount } = renderTab();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "1 of 3 connectors done · AWS prod: 3 steps done · last: ListAgentRuntimes (5 items)",
    );
    // Only the connectors still running or waiting are marked; OpenAI prod is done.
    expect(screen.getAllByText("Scanning")).toHaveLength(2);
    unmount();
    // One connector: its steps only; a step that failed shows its outcome.
    const one = runningScan().results[1];
    hoisted.summary = {
      totalFindings: 0,
      byProvider: {},
      onboarded: 0,
      connections: 3,
      latestScan: scan({
        status: "running",
        connectionIds: ["c2"],
        results: [{ ...one, calls: one.calls.slice(0, 2) }],
      }),
    };
    renderTab();
    const notice = screen.getByRole("alert");
    expect(notice).toHaveTextContent(/^Scanning AWS prod, started/);
    expect(notice).toHaveTextContent("2 steps done · last: ListKnowledgeBases (Permission denied)");
    expect(notice).not.toHaveTextContent("connectors done");
  });

  it("says a queued scan is waiting to start", () => {
    hoisted.connections = [connection()];
    hoisted.summary = {
      totalFindings: 0,
      byProvider: {},
      onboarded: 0,
      connections: 1,
      latestScan: scan({ status: "queued", results: [] }),
    };
    renderTab();
    expect(screen.getByRole("alert")).toHaveTextContent("Waiting to start");
  });

  it("cancels a running scan after confirming, only with the manage permission", () => {
    hoisted.connections = [connection()];
    hoisted.summary = {
      totalFindings: 0,
      byProvider: {},
      onboarded: 0,
      connections: 1,
      latestScan: scan({ status: "running" }),
    };
    const { unmount } = router(
      <ConnectionsTab orgName="acme" canManage={false} onAdd={vi.fn()} onEdit={vi.fn()} />,
    );
    expect(screen.queryByRole("button", { name: "Cancel scan" })).not.toBeInTheDocument();
    unmount();
    renderTab();
    fireEvent.click(within(screen.getByRole("alert")).getByRole("button", { name: "Cancel scan" }));
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.title).toBe("Cancel this scan?");
    expect(opts.description).toBe(
      "Agents already found stay. Connectors not finished yet are not scanned.",
    );
    expect(opts.cancelButtonText).toBe("Keep scanning");
    expect(hoisted.cancel).not.toHaveBeenCalled();
    opts.onConfirm();
    expect(hoisted.cancel).toHaveBeenCalledWith("s1", expect.anything());
  });

  it("says a scan it saw running was cancelled, not that it failed", () => {
    hoisted.connections = [
      connection(),
      connection({ id: "c2", name: "AWS prod", provider: "bedrock" }),
      connection({ id: "c3", name: "Google prod", provider: "google" }),
    ];
    const summary = (latestScan: ShadowScanResponse) => ({
      totalFindings: latestScan.findingsCount,
      byProvider: {},
      onboarded: 0,
      connections: 3,
      latestScan,
    });
    const ui = () => (
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <ConnectionsTab orgName="acme" canManage onAdd={vi.fn()} onEdit={vi.fn()} />
      </MemoryRouter>
    );
    hoisted.summary = summary(runningScan());
    const { rerender } = render(ui());
    hoisted.summary = summary(cancelledScan());
    rerender(ui());
    const notice = screen.getByRole("alert");
    expect(notice).toHaveTextContent(
      "Scan of 3 connectors cancelled: 2 agents found before it stopped.",
    );
    expect(notice).not.toHaveTextContent("failed");
    expect(within(notice).queryByRole("button", { name: "Cancel scan" })).not.toBeInTheDocument();
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
    expect(headers).toEqual(["Started", "Status", "Connectors scanned", "Agents found", ""]);
    expect(screen.queryByText(/History read|days of history/)).not.toBeInTheDocument();
    expect(screen.getAllByText("OpenAI prod").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: /View the scan started/ }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/shadow-agent-discovery/scans/s1",
    );
  });

  it("marks a scan recorded by a file upload", () => {
    hoisted.scans = [
      scan({
        inventoryUpload: true,
        connectionIds: ["c3"],
        results: [
          {
            connectionId: "c3",
            connectionName: "HR inventory",
            provider: "custom",
            outcome: "ok",
            findingsCount: 3,
            calls: [{ call: "Uploaded CSV file", outcome: "ok", items: 3 }],
          },
        ],
      }),
    ];
    router(<ScansTab orgName="acme" canManage />);
    expect(screen.getByText("HR inventory (file upload)")).toBeInTheDocument();
  });

  it("opens the scan's page when its row is clicked, with no drawer", () => {
    hoisted.scans = [scan()];
    router(<ScansTab orgName="acme" canManage />);
    fireEvent.click(screen.getAllByRole("row")[1]);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/shadow-agent-discovery/scans/s1",
    );
  });

  it("opens the unmanaged agents in the agent registry, even without the manage permission", () => {
    hoisted.scans = [scan()];
    router(<ScansTab orgName="acme" canManage={false} />);
    expect(screen.queryByRole("button", { name: "Upload results" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Clear scans" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "View unmanaged agents" }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/agent-registry?management=unmanaged",
    );
  });

  it("puts its actions in the slot the page gives, and no search above the table", () => {
    hoisted.scans = [scan()];
    const slot = document.createElement("div");
    document.body.appendChild(slot);
    router(<ScansTab orgName="acme" canManage actionsSlot={slot} />);
    const inSlot = within(slot);
    expect(inSlot.getByRole("button", { name: "View unmanaged agents" })).toBeInTheDocument();
    expect(inSlot.getByRole("button", { name: "Upload results" })).toBeInTheDocument();
    expect(inSlot.getByRole("button", { name: "Clear scans" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Upload results" })).toHaveLength(1);
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    slot.remove();
  });

  it("clears the finished scans after saying the agents are kept", () => {
    hoisted.scans = [scan()];
    router(<ScansTab orgName="acme" canManage />);
    fireEvent.click(screen.getByRole("button", { name: "Clear scans" }));
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.title).toBe("Clear all scans?");
    expect(opts.description).toContain("Unmanaged Agents and their evidence are kept");
    opts.onConfirm();
    expect(hoisted.clear).toHaveBeenCalledTimes(1);
  });

  it("keeps a pending scan when clearing", () => {
    hoisted.scans = [scan({ id: "s2", status: "pending" })];
    router(<ScansTab orgName="acme" canManage />);
    expect(screen.getByRole("button", { name: "Clear scans" })).toBeDisabled();
  });

  it("shows a running scan's progress and cancels it from its row", () => {
    hoisted.scans = [runningScan()];
    router(<ScansTab orgName="acme" canManage />);
    const row = screen.getAllByRole("row")[1];
    expect(within(row).getByText("Running")).toBeInTheDocument();
    expect(within(row).getByText("1 of 3 connectors done")).toBeInTheDocument();
    // A queued connector has no name yet: the row counts the connectors.
    expect(within(row).getByText("3 connectors")).toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: /Upload results/ })).not.toBeInTheDocument();
    fireEvent.click(within(row).getByRole("button", { name: /Cancel the scan started/ }));
    // Cancel does not also open the scan.
    expect(screen.getByTestId("location")).not.toHaveTextContent("/scans/");
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.description).toContain("Agents already found stay");
    opts.onConfirm();
    expect(hoisted.cancel).toHaveBeenCalledWith("s1", expect.anything());
  });

  it("offers Cancel for a queued scan's row, and none for a finished one", () => {
    hoisted.scans = [scan({ id: "s2", status: "queued", results: [] }), scan()];
    router(<ScansTab orgName="acme" canManage />);
    const [queued, finished] = screen.getAllByRole("row").slice(1);
    expect(within(finished).queryByRole("button", { name: /Cancel the scan/ })).toBeNull();
    fireEvent.click(within(queued).getByRole("button", { name: /Cancel the scan started/ }));
    hoisted.addConfirmation.mock.calls[0][0].onConfirm();
    expect(hoisted.cancel).toHaveBeenCalledWith("s2", expect.anything());
  });

  it("shows a cancelled scan as Cancelled", () => {
    hoisted.scans = [cancelledScan()];
    router(<ScansTab orgName="acme" canManage />);
    const row = screen.getAllByRole("row")[1];
    expect(within(row).getByText("Cancelled")).toBeInTheDocument();
    expect(within(row).queryByText("Failed")).not.toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: /Cancel the scan/ })).not.toBeInTheDocument();
  });
});

describe("ScanProgressCard", () => {
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

  it("says a file upload recorded the scan", () => {
    const upload = scan({
      inventoryUpload: true,
      connectionIds: ["c3"],
      results: [
        {
          connectionId: "c3",
          connectionName: "HR inventory",
          provider: "custom",
          outcome: "ok",
          findingsCount: 3,
          calls: [{ call: "Uploaded CSV file", outcome: "ok", items: 3 }],
        },
      ],
    });
    render(<ScanProgressCard scan={upload} connections={[]} />);
    expect(screen.getByText("Inventory file uploaded · 3 agents found.")).toBeInTheDocument();
  });

  it("shows the failed repositories with their reasons", () => {
    render(<ScanProgressCard scan={sourceScan()} connections={[]} />);
    expect(screen.getByText("Repositories: 2 scanned, 1 failed")).toBeInTheDocument();
    const failed = screen.getByRole("list", { name: "Failed repositories" });
    expect(within(failed).getByText("acme/big")).toBeInTheDocument();
    expect(within(failed).getByText("clone timed out")).toBeInTheDocument();
  });

  it("lists the scanned repositories on request", () => {
    render(<ScanProgressCard scan={sourceScan()} connections={[]} />);
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

  it("names a connection the scan has no name for from the connections", () => {
    render(
      <ScanProgressCard
        scan={runningScan()}
        connections={[connection({ id: "c3", name: "Inventory", provider: "custom" })]}
      />,
    );
    expect(screen.getByText("Inventory")).toBeInTheDocument();
    expect(screen.getByText("Your inventory · Not started")).toBeInTheDocument();
    expect(screen.queryByText("c3")).not.toBeInTheDocument();
    expect(screen.getByText("Waiting")).toBeInTheDocument();
  });

  it("stays after the scan ends: how long it took, what it found, and each connector's steps", () => {
    const finished = scan({
      startedAt: "2026-11-10T00:00:00Z",
      finishedAt: "2026-11-10T00:03:12Z",
      findingsCount: 3,
      results: [
        {
          connectionId: "c1",
          connectionName: "OpenAI prod",
          provider: "openai",
          outcome: "ok",
          findingsCount: 3,
          calls: [{ call: "GET /v1/agents", outcome: "ok", items: 3 }],
        },
      ],
    });
    render(<ScanProgressCard scan={finished} connections={[]} />);
    expect(screen.getByText("Progress")).toBeInTheDocument();
    expect(screen.getByText(/3 min 12 s taken$/)).toBeInTheDocument();
    expect(screen.getByText("1 connector scanned · 3 agents found.")).toBeInTheDocument();
    expect(screen.getByText("OpenAI · Done · 3 agents found")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show 1 step" }));
    const steps = screen.getByRole("list", { name: "Steps done" });
    expect(within(steps).getByText("GET /v1/agents")).toBeInTheDocument();
  });

  it("marks the connectors a cancel stopped as Cancelled", () => {
    render(<ScanProgressCard scan={cancelledScan()} connections={[]} />);
    // The scan's status and the two connectors that did not finish.
    expect(screen.getAllByText("Cancelled")).toHaveLength(3);
    expect(screen.queryByText("cancelled")).not.toBeInTheDocument();
    expect(
      screen.getByText(/^1 of 3 connectors finished before the scan was cancelled/),
    ).toBeInTheDocument();
  });

  it("shows no repositories for an AI platform", () => {
    render(<ScanProgressCard scan={scan()} connections={[]} />);
    expect(screen.getByText("OpenAI prod")).toBeInTheDocument();
    expect(screen.queryByText(/^Repositories/)).not.toBeInTheDocument();
  });
});

describe("ScanStatusChip", () => {
  it("shows a scan waiting for its scan command as Waiting for results", () => {
    render(<ScanStatusChip status="pending" />);
    expect(screen.getByText("Waiting for results")).toBeInTheDocument();
  });
});

describe("ScanFindings (scan page)", () => {
  const scanPage = (canManage = true) =>
    router(
      <Routes>
        <Route
          path="/organizations/:orgId/shadow-agent-discovery/scans/:scanId"
          element={
            <ScanFindings
              orgName="acme"
              listPath="/organizations/acme/shadow-agent-discovery"
              canManage={canManage}
            />
          }
        />
      </Routes>,
      "/organizations/acme/shadow-agent-discovery/scans/s1",
    );

  it("shows a running scan's progress, one row per connector", () => {
    hoisted.connections = [connection({ id: "c3", name: "Inventory", provider: "custom" })];
    hoisted.scanDetail = runningScan();
    scanPage();
    expect(screen.getByText("Progress")).toBeInTheDocument();
    expect(screen.getByText("1 of 3 connectors done.", { exact: false })).toBeInTheDocument();
    expect(screen.getByText(/elapsed$/)).toBeInTheDocument();
    const rows = within(screen.getByRole("list", { name: "Connectors" })).getAllByRole("listitem");
    expect(rows).toHaveLength(3);
    const [done, scanning, waiting] = rows;
    expect(within(done).getByText("OpenAI prod")).toBeInTheDocument();
    expect(within(done).getByText("OK")).toBeInTheDocument();
    expect(within(done).getByText("OpenAI · Done · 2 agents found")).toBeInTheDocument();
    expect(within(scanning).getByText("Scanning")).toBeInTheDocument();
    expect(
      within(scanning).getByText("Last step: ListAgentRuntimes (5 items)"),
    ).toBeInTheDocument();
    // A queued connector has no name in the scan yet: it comes from the connectors.
    expect(within(waiting).getByText("Inventory")).toBeInTheDocument();
    expect(within(waiting).getByText("Waiting")).toBeInTheDocument();
  });

  it("refreshes the scan's agents when a connector finishes or the scan ends, not on each poll", () => {
    hoisted.scanDetail = runningScan();
    const ui = () => (
      <MemoryRouter
        initialEntries={["/organizations/acme/shadow-agent-discovery/scans/s1"]}
        future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
      >
        <Routes>
          <Route
            path="/organizations/:orgId/shadow-agent-discovery/scans/:scanId"
            element={<ScanFindings orgName="acme" listPath="/x" canManage />}
          />
        </Routes>
      </MemoryRouter>
    );
    const { rerender } = render(ui());
    expect(hoisted.refreshFindings).not.toHaveBeenCalled();
    // A poll with one more step done: same connectors done.
    const polled = runningScan();
    polled.results[1].calls = [...polled.results[1].calls, { call: "X", outcome: "ok", items: 1 }];
    hoisted.scanDetail = polled;
    rerender(ui());
    expect(hoisted.refreshFindings).not.toHaveBeenCalled();
    // AWS prod finished.
    const next = runningScan();
    next.results[1] = { ...next.results[1], outcome: "ok", findingsCount: 4 };
    hoisted.scanDetail = next;
    rerender(ui());
    expect(hoisted.refreshFindings).toHaveBeenCalledTimes(1);
    // The scan ended.
    hoisted.scanDetail = { ...next, status: "succeeded" };
    rerender(ui());
    expect(hoisted.refreshFindings).toHaveBeenCalledTimes(2);
  });

  it("lists the steps a connector finished on request", () => {
    hoisted.scanDetail = runningScan();
    scanPage();
    expect(screen.queryByRole("list", { name: "Steps done" })).not.toBeInTheDocument();
    const toggle = screen.getByRole("button", { name: "Show 3 steps done" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(toggle);
    const steps = screen.getByRole("list", { name: "Steps done" });
    expect(toggle).toHaveAttribute("aria-controls", steps.id);
    expect(within(steps).getAllByRole("listitem")).toHaveLength(3);
    expect(within(steps).getByText("Permission denied")).toBeInTheDocument();
  });

  it("cancels a running scan after confirming, only with the manage permission", () => {
    hoisted.scanDetail = runningScan();
    const { unmount } = scanPage(false);
    expect(screen.queryByRole("button", { name: "Cancel scan" })).not.toBeInTheDocument();
    unmount();
    scanPage();
    fireEvent.click(screen.getByRole("button", { name: "Cancel scan" }));
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.title).toBe("Cancel this scan?");
    opts.onConfirm();
    expect(hoisted.cancel).toHaveBeenCalledWith("s1", expect.anything());
  });

  it("keeps the progress of a cancelled scan, with no Cancel scan or Scan details", () => {
    hoisted.scanDetail = cancelledScan();
    scanPage();
    expect(screen.getByText("Progress")).toBeInTheDocument();
    expect(
      screen.getByText(/^1 of 3 connectors finished before the scan was cancelled/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cancel scan" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Scan details" })).not.toBeInTheDocument();
  });

  it("keeps the pending scan's cancel wording", () => {
    hoisted.scanDetail = scan({ status: "pending", results: [] });
    scanPage();
    fireEvent.click(screen.getByRole("button", { name: "Cancel scan" }));
    const opts = hoisted.addConfirmation.mock.calls[0][0];
    expect(opts.description).toContain("stops waiting for results");
    expect(opts.cancelButtonText).toBe("Keep waiting");
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
    render(
      <AnalyzeSection
        providerName="Amazon Bedrock"
        statuses={statuses}
        notListed={["Classic agent instructions and prompt overrides."]}
      />,
    );
    expect(screen.getByText("What does Agent Manager list?")).toBeInTheDocument();
    expect(
      screen.getByText(
        "AWS Agent Registry agent records, AgentCore runtimes, with read-only calls.",
      ),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "See what it lists" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent("What Agent Manager lists");
    expect(dialog).toHaveTextContent("A scan makes only these read-only calls to Amazon Bedrock.");
    expect(dialog).toHaveTextContent("Agent records in each registry.");
    expect(dialog).toHaveTextContent("Agent runtimes in each region.");
    expect(dialog).toHaveTextContent("Not read or listedClassic agent instructions and prompt overrides.");
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

  it("offers AI-enhanced detection with no LLM judge and no AI warning", () => {
    render(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={vi.fn()} />,
    );
    fireEvent.click(screen.getByRole("switch", { name: /^AI-enhanced detection/ }));
    expect(screen.queryByRole("switch", { name: /LLM judge/ })).not.toBeInTheDocument();
    expect(screen.queryByText(/second model/)).not.toBeInTheDocument();
    expect(screen.queryByText(/can make mistakes/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    expect(hoisted.generate).toHaveBeenLastCalledWith(
      { connectionId: "c2", aiReview: true, llmJudge: false, directUpload: false },
      expect.anything(),
    );
  });

  it("shows the command, composed from the saved settings and the issued scan", () => {
    render(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={vi.fn()} />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    expect(hoisted.generate).toHaveBeenLastCalledWith(
      { connectionId: "c2", aiReview: false, llmJudge: false, directUpload: false },
      expect.anything(),
    );
    const opts = hoisted.generate.mock.lastCall?.[1] as MutateOptions;
    // Without direct upload no token is issued.
    act(() => opts.onSuccess(issuedScan({ token: undefined, expiresAt: undefined })));
    // The command as one line: its " \\" line continuations joined.
    const code = (screen.getByText(/--scan-id s9/).textContent ?? "").replace(/ \\\n\s*/g, " ");
    expect(code).not.toMatch(/AMP_URL|AMP_SCAN_TOKEN/);
    expect(code).toContain("ghcr.io/wso2/scanner:1.0 scan --scan-id s9");
    expect(code).toContain("--github-org wso2 --all-repos");
    expect(screen.getByText(/shows as Waiting for results until you upload/)).toBeInTheDocument();
    expect(screen.queryByText(/Shown once/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Generate" })).not.toBeInTheDocument();
  });

  it("starts with direct upload off, and with it on issues a command with a token", () => {
    render(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={vi.fn()} />,
    );
    const direct = screen.getByRole("switch", { name: /^Upload results directly/ });
    expect(direct).not.toBeChecked();
    expect(screen.getByText(/saves a results file on your machine/)).toBeInTheDocument();
    fireEvent.click(direct);
    expect(screen.getByText(/asks to upload the results with a one-hour/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Generate" }));
    expect(hoisted.generate).toHaveBeenLastCalledWith(
      { connectionId: "c2", aiReview: false, llmJudge: false, directUpload: true },
      expect.anything(),
    );
    const opts = hoisted.generate.mock.lastCall?.[1] as MutateOptions;
    act(() => opts.onSuccess(issuedScan()));
    const code = (screen.getByText(/AMP_SCAN_TOKEN=tok-1/).textContent ?? "").replace(/ \\\n\s*/g, " ");
    expect(code).toContain("-e AMP_URL=https://amp.example.com");
    expect(code).toContain("ghcr.io/wso2/scanner:1.0 scan --scan-id s9");
    expect(screen.getByText(/Shown once: copy it now/)).toBeInTheDocument();
  });

  it("never shows a command again after closing, and closes only with Close", () => {
    const onClose = vi.fn();
    const { rerender } = render(
      <GenerateCommandDialog orgName="acme" target={target} spec={spec} onClose={onClose} />,
    );
    fireEvent.click(screen.getByRole("switch", { name: /^Upload results directly/ }));
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
    // Reopened, direct upload is off again.
    expect(screen.getByRole("switch", { name: /^Upload results directly/ })).not.toBeChecked();
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

  const openPicker = (providers: ShadowProviderSpec[]) => {
    hoisted.providers = providers;
    router(
      <Routes>
        <Route path="/new" element={<ConnectionEditor orgName="acme" listPath="/list" />} />
      </Routes>,
      "/new",
    );
  };
  const aiPlatform = (provider: ShadowProvider, displayName: string): ShadowProviderSpec =>
    sourceHostSpec({ provider, displayName, sourceHost: false, permissions: [], docs: [] });

  it("groups the platforms to add into AI platforms and Source code, with logos and tags", () => {
    openPicker([
      githubSpec(),
      aiPlatform("anthropic", "Anthropic"),
      aiPlatform("openai", "OpenAI"),
    ]);
    const ai = screen.getByRole("region", { name: "AI platforms" });
    const source = screen.getByRole("region", { name: "Source code repositories" });
    // In display order, whatever order the server lists them in.
    expect(within(ai).getAllByRole("button").map((b) => b.getAttribute("aria-label"))).toEqual([
      "Connect OpenAI",
      "Connect Anthropic",
    ]);
    expect(within(ai).getAllByText("Read-only key")).toHaveLength(2);
    expect(
      screen.getByRole("button", { name: "Connect OpenAI" }),
    ).toHaveAccessibleDescription("Assistants and agents in your OpenAI projects");
    expect(within(source).getByRole("button", { name: "Connect GitHub" })).toBeInTheDocument();
    expect(within(source).getByText("Runs on your machine")).toBeInTheDocument();
    expect(within(source).getByText("Agents in your repositories' code")).toBeInTheDocument();
  });

  it("chooses a platform card from the keyboard", () => {
    openPicker([githubSpec(), aiPlatform("openai", "OpenAI")]);
    const card = screen.getByRole("button", { name: "Connect GitHub" });
    expect(card).toHaveAttribute("tabindex", "0");
    act(() => card.focus());
    expect(card).toHaveFocus();
    fireEvent.keyDown(card, { key: "Enter" });
    expect(screen.getByText("Scan command")).toBeInTheDocument();
  });

  it("chooses a platform card with Space", () => {
    openPicker([aiPlatform("openai", "OpenAI")]);
    const card = screen.getByRole("button", { name: "Connect OpenAI" });
    act(() => card.focus());
    fireEvent.keyDown(card, { key: " " });
    fireEvent.keyUp(card, { key: " " });
    expect(screen.queryByRole("button", { name: "Connect OpenAI" })).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /Name/ })).toHaveValue("OpenAI connector");
  });

  it("adds --ai-review with AI-enhanced detection, and never offers the LLM judge", () => {
    openNew();
    const ai = screen.getByRole("switch", { name: /^AI-enhanced detection/ });
    fireEvent.click(ai);
    expect(commandText()).toContain("--ai-review");
    expect(screen.queryByRole("switch", { name: /LLM judge/ })).not.toBeInTheDocument();
    expect(commandText()).not.toContain("--judge");
    fireEvent.click(ai);
    expect(commandText()).not.toContain("--ai-review");
  });

  it("shows the scan command next to the form, following the organization field", () => {
    openNew();
    expect(screen.getByText("Scan command")).toBeInTheDocument();
    expect(commandText()).toContain("--github-org <organization> --repos=<repositories>");
    // Direct upload starts off: no Agent Manager address or token.
    expect(screen.getByRole("switch", { name: /^Upload results directly/ })).not.toBeChecked();
    expect(commandText()).not.toMatch(/AMP_URL|AMP_SCAN_TOKEN/);
    fireEvent.click(screen.getByRole("switch", { name: /^Upload results directly/ }));
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
    fireEvent.click(screen.getByRole("switch", { name: /^AI-enhanced detection/ }));
    fireEvent.click(screen.getByRole("switch", { name: /^Upload results directly/ }));
    fireEvent.click(screen.getByRole("button", { name: "Add source and get command" }));
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
      { connectionId: "c9", aiReview: true, llmJudge: false, directUpload: true },
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
      screen.queryByRole("button", { name: "Add source and get command" }),
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
    fireEvent.click(screen.getByRole("button", { name: "Add source and get command" }));
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
      { connectionId: "c9", aiReview: false, llmJudge: false, directUpload: false },
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
    fireEvent.click(screen.getByRole("button", { name: "Add source and get command" }));
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

describe("ConnectionEditor (Bring your own inventory)", () => {
  const openNew = () => {
    hoisted.providers = [inventorySpec()];
    router(
      <Routes>
        <Route path="/new" element={<ConnectionEditor orgName="acme" listPath="/list" />} />
        <Route path="/list" element={<div>Sources list</div>} />
      </Routes>,
      "/new",
    );
    fireEvent.click(screen.getByRole("button", { name: "Connect Bring your own inventory" }));
  };
  const choose = (file: File) =>
    fireEvent.change(screen.getByTestId("inventory-file-input"), { target: { files: [file] } });
  const jsonFile = () => new File(['{"agents":[]}'], "agents.json", { type: "application/json" });

  it("shows Bring your own inventory as its own group when adding a platform", () => {
    hoisted.providers = [
      { ...inventorySpec(), provider: "openai", displayName: "OpenAI", fileUpload: false },
      inventorySpec(),
      sourceHostSpec(),
    ];
    router(
      <Routes>
        <Route path="/new" element={<ConnectionEditor orgName="acme" listPath="/list" />} />
      </Routes>,
      "/new",
    );
    const groups = screen
      .getAllByRole("heading", { name: /^(AI platforms|Bring your own inventory|Source code repositories)$/ })
      .map((h) => h.textContent);
    expect(groups).toEqual(["AI platforms", "Source code repositories", "Bring your own inventory"]);
    const custom = screen.getByRole("region", { name: "Bring your own inventory" });
    expect(
      within(custom).getByRole("button", { name: "Connect Bring your own inventory" }),
    ).toHaveAccessibleDescription(
      "Agents listed in a JSON, CSV or Excel file you upload, in the Agent Manager format",
    );
    expect(within(custom).getByText("File upload")).toBeInTheDocument();
  });

  it("asks for a name and a file only: no URL, key, contract or connection test", () => {
    openNew();
    expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("My agent inventory");
    expect(screen.getByText("Download a template and list your agents in it.")).toBeInTheDocument();
    expect(screen.queryByText(/Agents URL|API key|contract/i)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Download contract|See what it lists/ }))
      .not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add source" })).toBeInTheDocument();
  });

  it("shows each format's fields, an example and its template", () => {
    openNew();
    const tabs = screen.getAllByRole("tab").map((t) => t.textContent);
    expect(tabs).toEqual(["JSON", "CSV", "Excel"]);
    const fields = screen.getByRole("table", { name: "Inventory fields" });
    expect(within(fields).getAllByRole("row")).toHaveLength(5);
    expect(within(fields).getByText("labels")).toBeInTheDocument();
    // JSON: labels are an object, and the example is a whole file.
    expect(within(fields).getByText('{"team":"support","region":"eu"}')).toBeInTheDocument();
    expect(screen.getByLabelText("Example JSON file")).toHaveTextContent('"agents": [');
    expect(screen.getByText(/an agents array, one object per agent/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Download template" }));
    expect(hoisted.downloadTemplate).toHaveBeenLastCalledWith("json");

    fireEvent.click(screen.getByRole("tab", { name: "CSV" }));
    expect(screen.getByText(/first row is the column names/)).toBeInTheDocument();
    expect(screen.getByLabelText("Example CSV file").textContent).toBe(
      'id,name,owner,labels\nhr-bot,"HR, ""helper""",Ana,team=support; region=eu',
    );
    expect(
      within(screen.getByRole("table", { name: "Inventory fields" })).getByText(
        "team=support; region=eu",
      ),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Download template" }));
    expect(hoisted.downloadTemplate).toHaveBeenLastCalledWith("csv");

    fireEvent.click(screen.getByRole("tab", { name: "Excel" }));
    const sheet = screen.getByRole("table", { name: "Example Excel sheet" });
    expect(within(sheet).getByText("hr-bot")).toBeInTheDocument();
    expect(within(sheet).getByText("D")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Download template" }));
    expect(hoisted.downloadTemplate).toHaveBeenLastCalledWith("xlsx");
  });

  it("rejects a file of another kind or over 5 MB, and opens the tab of a known kind", () => {
    openNew();
    choose(new File(["x"], "agents.txt"));
    expect(screen.getByText("Choose a .json, .csv or .xlsx file.")).toBeInTheDocument();
    const big = new File(["x"], "agents.json");
    Object.defineProperty(big, "size", { value: 5 * 1024 * 1024 + 1 });
    choose(big);
    expect(
      screen.getByText("This file is larger than 5 MB, the most Agent Manager accepts."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add source" })).toBeInTheDocument();
    // A CSV chosen on the JSON tab opens the CSV tab.
    choose(new File(["id,name\n"], "agents.csv", { type: "text/csv" }));
    expect(screen.getByRole("tab", { name: "CSV" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("agents.csv")).toBeInTheDocument();
    expect(screen.queryByText(/Choose a \.json/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Remove agents.csv" }));
    expect(screen.queryByText("agents.csv")).not.toBeInTheDocument();
  });

  it("saves the source, uploads the file to it and shows what it gave", () => {
    openNew();
    const file = jsonFile();
    choose(file);
    fireEvent.click(screen.getByRole("button", { name: "Add source and upload" }));
    expect(hoisted.createConnection).toHaveBeenCalledWith(
      { name: "My agent inventory", provider: "custom", endpoints: {}, settings: {} },
      expect.anything(),
    );
    act(() => hoisted.createConnection.mock.calls[0][1].onSuccess(connection({ id: "c9" })));
    expect(hoisted.uploadInventory).toHaveBeenCalledWith(
      { connectionId: "c9", file },
      expect.anything(),
    );
    act(() => hoisted.uploadInventory.mock.calls[0][1].onSuccess(uploadResult()));
    const result = screen.getByRole("alert");
    expect(result).toHaveTextContent("12 agents saved, 1 agent left out.");
    expect(result).toHaveTextContent("Ignored column (not stored): secret");
    fireEvent.click(within(result).getByRole("button", { name: "Show details" }));
    expect(within(result).getByText("Left out")).toBeInTheDocument();
    expect(within(result).getByText("Agent 4: no name")).toBeInTheDocument();
    fireEvent.click(within(result).getByRole("button", { name: "View scan" }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/shadow-agent-discovery/scans/s-up",
    );
  });

  it("links to the agents found, and Done returns to the sources", () => {
    openNew();
    choose(jsonFile());
    fireEvent.click(screen.getByRole("button", { name: "Add source and upload" }));
    act(() => hoisted.createConnection.mock.calls[0][1].onSuccess(connection({ id: "c9" })));
    act(() => hoisted.uploadInventory.mock.calls[0][1].onSuccess(uploadResult()));
    expect(screen.getByRole("textbox", { name: "Name" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.getByText("Sources list")).toBeInTheDocument();
  });

  it("shows why an upload failed and lets the user upload again to the saved source", () => {
    openNew();
    choose(jsonFile());
    fireEvent.click(screen.getByRole("button", { name: "Add source and upload" }));
    act(() => hoisted.createConnection.mock.calls[0][1].onSuccess(connection({ id: "c9" })));
    act(() =>
      hoisted.uploadInventory.mock.calls[0][1].onError(
        Object.assign(new Error("The file has no name column."), { status: 400 }),
      ),
    );
    expect(
      screen.getByText("The source is saved. The file has no name column."),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Upload" }));
    expect(hoisted.uploadInventory).toHaveBeenCalledTimes(2);
    expect(hoisted.createConnection).toHaveBeenCalledTimes(1);
    expect(hoisted.uploadInventory.mock.calls[1][0].connectionId).toBe("c9");
  });

  it("keeps the chosen file when another tab is shown", () => {
    openNew();
    choose(new File(["id,name\n"], "agents.csv", { type: "text/csv" }));
    fireEvent.click(screen.getByRole("tab", { name: "JSON" }));
    // The file's name and its format chip.
    const chosen = screen.getByText("agents.csv").parentElement as HTMLElement;
    expect(within(chosen).getByText("CSV")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add source and upload" })).toBeInTheDocument();
  });

  it("splits rows left out from warnings, in the uploaded file's wording", () => {
    openNew();
    choose(new File(["id,name\n"], "agents.csv", { type: "text/csv" }));
    // The JSON tab is shown, but the file is CSV: rows, not agents.
    fireEvent.click(screen.getByRole("tab", { name: "JSON" }));
    fireEvent.click(screen.getByRole("button", { name: "Add source and upload" }));
    act(() => hoisted.createConnection.mock.calls[0][1].onSuccess(connection({ id: "c9" })));
    act(() =>
      hoisted.uploadInventory.mock.calls[0][1].onSuccess({
        ...uploadResult(),
        agentsSaved: 10,
        leftOut: 1,
        problems: [
          { row: 4, message: "no name", leftOut: true },
          { row: 7, message: "createdAt is not a date; left empty", leftOut: false },
        ],
        problemCount: 2,
      }),
    );
    const result = screen.getByRole("alert");
    expect(result).toHaveTextContent(
      "10 agents saved, 1 row left out. 1 row saved with a warning.",
    );
    fireEvent.click(within(result).getByRole("button", { name: "Show details" }));
    expect(within(result).getByText("Row 4: no name")).toBeInTheDocument();
    expect(within(result).getByText("Saved with a warning")).toBeInTheDocument();
    expect(
      within(result).getByText("Row 7: createdAt is not a date; left empty"),
    ).toBeInTheDocument();
    expect(within(result).queryByText(/Showing the first/)).not.toBeInTheDocument();
  });

  it("says when the server sent only the first problems", () => {
    openNew();
    choose(jsonFile());
    fireEvent.click(screen.getByRole("button", { name: "Add source and upload" }));
    act(() => hoisted.createConnection.mock.calls[0][1].onSuccess(connection({ id: "c9" })));
    act(() =>
      hoisted.uploadInventory.mock.calls[0][1].onSuccess({
        ...uploadResult(),
        leftOut: 80,
        problemCount: 80,
        problems: Array.from({ length: 50 }, (_v, i) => ({
          row: i + 1,
          message: "no name",
          leftOut: true,
        })),
      }),
    );
    const result = screen.getByRole("alert");
    expect(result).toHaveTextContent("12 agents saved, 80 agents left out.");
    fireEvent.click(within(result).getByRole("button", { name: "Show details" }));
    expect(within(result).getByText("Showing the first 50 of 80.")).toBeInTheDocument();
  });

  it("shows the smaller size limit when the deployment caps request bodies", async () => {
    const { globalConfig } = await import("@agent-management-platform/types");
    const config = globalConfig as unknown as Record<string, unknown>;
    const saved = config.maxRequestBodyBytes;
    config.maxRequestBodyBytes = 57344;
    try {
      openNew();
      expect(screen.getByText("Up to 56 KB and 5,000 agents")).toBeInTheDocument();
      const big = new File(["x"], "agents.json");
      Object.defineProperty(big, "size", { value: 57345 });
      choose(big);
      expect(
        screen.getByText("This file is larger than 56 KB, the most Agent Manager accepts."),
      ).toBeInTheDocument();
    } finally {
      config.maxRequestBodyBytes = saved;
    }
  });

  it("adds a source without a file and returns to the sources", () => {
    openNew();
    fireEvent.click(screen.getByRole("button", { name: "Add source" }));
    act(() => hoisted.createConnection.mock.calls[0][1].onSuccess(connection({ id: "c9" })));
    expect(hoisted.uploadInventory).not.toHaveBeenCalled();
    expect(screen.getByText("Sources list")).toBeInTheDocument();
  });
});

describe("inventory file helpers", () => {
  it("explains a superseded upload and an audit outage", () => {
    const superseded =
      "A newer file was uploaded to this source while this one was being saved. " +
      "Upload it again if it is the one you want.";
    expect(
      inventoryUploadErrorMessage(Object.assign(new Error(superseded), { status: 409 })),
    ).toBe(superseded);
    expect(inventoryUploadErrorMessage(Object.assign(new Error("x"), { status: 503 }))).toBe(
      "Agent Manager couldn't record the upload in the audit log, so nothing was saved. " +
        "Try again later.",
    );
  });

  it("never takes an upload for the latest cloud scan", () => {
    expect(latestCloudScan({ latestScan: scan({ inventoryUpload: true }) })).toBeUndefined();
    expect(latestCloudScan({ latestScan: scan() })?.id).toBe("s1");
    expect(latestCloudScan(undefined)).toBeUndefined();
  });

  it("knows a file's format by its extension", () => {
    expect(inventoryFormatOf("a.JSON")).toBe("json");
    expect(inventoryFormatOf("a.csv")).toBe("csv");
    expect(inventoryFormatOf("a.xlsx")).toBe("xlsx");
    expect(inventoryFormatOf("a.xls")).toBeUndefined();
    expect(inventoryFormatOf("xlsx")).toBeUndefined();
  });

  it("builds the examples from the fields", () => {
    const fields = inventorySpec().inventoryFields ?? [];
    expect(JSON.parse(exampleJson(fields))).toEqual({
      agents: [
        {
          id: "hr-bot",
          name: 'HR, "helper"',
          owner: "Ana",
          labels: { team: "support", region: "eu" },
        },
      ],
    });
    expect(exampleCsv(fields).split("\n")[0]).toBe("id,name,owner,labels");
    expect(csvValue("plain")).toBe("plain");
    expect(csvValue('a "b"')).toBe('"a ""b"""');
  });
});

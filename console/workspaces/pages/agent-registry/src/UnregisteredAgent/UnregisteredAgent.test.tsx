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
  useUploadShadowScanResults: () => ({
    mutate: hoisted.upload,
    reset: hoisted.resetUpload,
    isPending: false,
  }),
  isRequestTooLargeError: () => false,
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

import { FeatureManagementProvider } from "@agent-management-platform/feature-management";
import { FindingDetail, parseCodeLocation, relationLabel } from "./FindingDetail";
import { OnboardFinding } from "./OnboardFinding";
import { buildAgentSourceSeed, folderAppPath, seedAppPath, seedLanguage } from "./platformHostedSeed";

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


describe("FindingDetail", () => {
  const renderDetail = (canManage = true) =>
    router(
      <Routes>
        <Route
          path="/organizations/:orgId/agent-registry/unregistered/:findingId"
          element={
            <FindingDetail
              orgName="acme"
              canManage={canManage}
              listPath="/organizations/acme/agent-registry?registration=unregistered"
            />
          }
        />
      </Routes>,
      "/organizations/acme/agent-registry/unregistered/f1",
    );
  const sidebar = () => screen.getByRole("complementary", { name: "Properties" });

  it("shows why it is an agent, what it uses and its status, and offers onboarding", () => {
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
    expect(screen.getByText("proj_1")).toBeInTheDocument();
    expect(screen.queryByLabelText("Location in code")).not.toBeInTheDocument();
    expect(screen.getAllByText("Not onboarded").length).toBeGreaterThan(0);
    for (const section of ["Why this is an agent", "What it uses", "Status", "Technical IDs"]) {
      expect(screen.getByText(section)).toBeInTheDocument();
    }
    expect(screen.getByText("Agent object")).toBeInTheDocument();
    expect(screen.getByText("Returned by GET /v1/agents")).toBeInTheDocument();
    expect(screen.getByText("Listed by the OpenAI API.")).toBeInTheDocument();
    expect(screen.queryByText(/Confirmed by/)).not.toBeInTheDocument();
    expect(screen.getByText("Show listing details")).toBeInTheDocument();
    expect(screen.queryByText("Show matched code")).not.toBeInTheDocument();
    // The raw evidence is behind Show listing details, and only rendered once opened.
    for (const name of ["Show listing details", "Technical IDs"]) {
      expect(screen.getByRole("button", { name })).toHaveAttribute("aria-expanded", "false");
    }
    expect(screen.queryByText(/an item in data/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Show listing details" }));
    expect(screen.getByText(/an item in data/)).toBeInTheDocument();
    expect(screen.getByText("toolCount: 2")).toBeInTheDocument();
    expect(screen.getByText("openai.agent_object")).toBeInTheDocument();
    expect(screen.getByText("Model")).toBeInTheDocument();
    expect(screen.getByText("gpt-4.1")).toBeInTheDocument();
    expect(screen.getByText("function · lookup_order")).toBeInTheDocument();
    expect(screen.getByText("github · https://mcp.example.com")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Technical IDs" }));
    expect(screen.getByText("Platform ID")).toBeInTheDocument();
    expect(screen.getByText("agent_abc")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Found by 1 connection")).toBeInTheDocument();
    expect(within(sidebar()).getByText("OpenAI prod")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Listed")).toBeInTheDocument();
    expect(screen.queryByLabelText(/merged into one agent/)).not.toBeInTheDocument();
    expect(within(sidebar()).getByText("active")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Last found")).toBeInTheDocument();
    expect(within(sidebar()).getByText("First found")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /from list|in list/ })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Onboard as external agent" })).toHaveAttribute(
      "href",
      "/organizations/acme/agent-registry/unregistered/f1/onboard",
    );
    expect(screen.queryByText(/score|verdict|reviewed|dismiss/i)).not.toBeInTheDocument();
    expect(screen.queryByText("No longer found")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByText("Might be the same agent")).not.toBeInTheDocument();
    expect(screen.queryByText(/can make mistakes/)).not.toBeInTheDocument();
    // Both were opened above.
    expect(screen.getByRole("button", { name: "Show listing details" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByRole("button", { name: "Technical IDs" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  const codeAgent = (reviewedBy: string, over: Partial<ShadowFindingDetail> = {}) =>
    detail({
      provider: "github",
      kind: "code_agent",
      scope: "support-bot",
      externalId: "support-bot:samples/support/agent.py:SupportAgent",
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

  it("shows a code agent's location as a breadcrumb, with a copy button", () => {
    hoisted.detail = codeAgent("rules");
    renderDetail();
    expect(screen.getByText("Code agent on GitHub")).toBeInTheDocument();
    const crumbs = screen.getByRole("navigation", { name: "Location in code" });
    expect(within(crumbs).getByText("support-bot")).toBeInTheDocument();
    expect(within(crumbs).getByText("samples/support/agent.py")).toBeInTheDocument();
    expect(within(crumbs).getByText("SupportAgent")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy location" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Technical IDs" }));
    expect(screen.getByText("Location in code")).toBeInTheDocument();
    expect(screen.queryByText("Platform ID")).not.toBeInTheDocument();
    expect(screen.getByText("In requirements.txt")).toBeInTheDocument();
    expect(screen.getByText("Show matched code")).toBeInTheDocument();
    expect(screen.getByText(/What it uses/)).toBeInTheDocument();
    expect(screen.getByText(/Reported by the scan command/)).toBeInTheDocument();
    expect(within(sidebar()).getByText("Found")).toBeInTheDocument();
    expect(screen.queryByText("State")).not.toBeInTheDocument();
    expect(screen.queryByText("Created at the platform")).not.toBeInTheDocument();
  });

  it("parses a code location, including an @N repeat and a location without a name", () => {
    expect(parseCodeLocation("support-bot:samples/support/agent.py:SupportAgent@2")).toEqual({
      repository: "support-bot",
      path: "samples/support/agent.py",
      symbol: "SupportAgent",
      occurrence: 2,
    });
    expect(parseCodeLocation("bot:app/main.py:Router.build")).toEqual({
      repository: "bot",
      path: "app/main.py",
      symbol: "Router.build",
    });
    expect(parseCodeLocation("wso2/support-bot:agent.py")).toEqual({
      repository: "wso2/support-bot",
      path: "agent.py",
      symbol: "",
    });
  });

  it("shows the name without its @N suffix, and copies the full location", () => {
    hoisted.detail = codeAgent("rules", {
      externalId: "support-bot:samples/support/agent.py:SupportAgent@2",
    });
    renderDetail();
    const crumbs = screen.getByRole("navigation", { name: "Location in code" });
    expect(within(crumbs).getByText("SupportAgent")).toBeInTheDocument();
    expect(within(crumbs).queryByText(/@2/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Technical IDs" }));
    expect(
      screen.getByText("support-bot:samples/support/agent.py:SupportAgent@2"),
    ).toBeInTheDocument();
  });

  it.each([
    ["rules", "Confirmed by detection rules", false],
    ["ai_review", "Confirmed by AI review", true],
    ["ai_review_judge", "Confirmed by AI review, checked by a second model", true],
  ])("words who confirmed a code agent (%s)", (reviewedBy, text, ai) => {
    hoisted.detail = codeAgent(reviewedBy);
    renderDetail();
    expect(screen.getByText(text)).toBeInTheDocument();
    expect(screen.queryByText("Confirmed by")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    // The AI caution is visible text, shown once, not hidden in a tooltip.
    const caution = screen.queryAllByText("AI review can make mistakes, so check the evidence.");
    expect(caution).toHaveLength(ai ? 1 : 0);
    expect(screen.queryByLabelText(/can make mistakes/)).not.toBeInTheDocument();
  });

  it("shows only the repository of a location the runner capped", () => {
    expect(parseCodeLocation(`support-bot:${"a/".repeat(20)}agent.py#0123456789ab`)).toEqual({
      repository: "support-bot",
      path: "",
      symbol: "",
    });
    const capped = `support-bot:${"a/".repeat(20)}agent.py:Sup#0123456789ab`;
    hoisted.detail = codeAgent("rules", { externalId: capped });
    renderDetail();
    const crumbs = screen.getByRole("navigation", { name: "Location in code" });
    expect(within(crumbs).getByText("support-bot")).toBeInTheDocument();
    expect(within(crumbs).queryByText(/agent\.py|#0123456789ab/)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy location" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Technical IDs" }));
    expect(screen.getByText(capped)).toBeInTheDocument();
  });

  it("labels branch, folder and language, and drops a folder the location already shows", () => {
    hoisted.detail = codeAgent("rules", {
      attributes: {
        framework: "langgraph",
        branch: "develop",
        language: "python",
        projectPath: "/samples/support",
        reviewedBy: "rules",
        description: "",
      },
    });
    const { unmount } = renderDetail();
    for (const label of ["Framework", "Branch", "Language"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
    expect(screen.getByText("develop")).toBeInTheDocument();
    expect(screen.getByText("python")).toBeInTheDocument();
    expect(screen.queryByText("Folder")).not.toBeInTheDocument();
    expect(screen.queryByText("Description")).not.toBeInTheDocument();
    expect(screen.queryByText(/^(branch|language|projectPath|reviewedBy)$/)).toBeNull();
    unmount();

    hoisted.detail = codeAgent("rules", { attributes: { projectPath: "/services/bot" } });
    renderDetail();
    expect(screen.getByText("Folder")).toBeInTheDocument();
    expect(screen.getByText("/services/bot")).toBeInTheDocument();
  });

  it("says so when the listing reports nothing more", () => {
    hoisted.detail = codeAgent("rules", { attributes: {} });
    renderDetail();
    expect(screen.getByText("Nothing more was reported about this agent.")).toBeInTheDocument();
  });

  it("shows one short notice and a chip when no connection finds the agent any more", () => {
    hoisted.detail = codeAgent("ai_review", {
      present: false,
      foundBy: [
        { findingId: "f1", connectionId: "c1", connectionName: "WSO2 GitHub", present: false },
        { findingId: "f9", connectionId: "c9", connectionName: "WSO2 mirror", present: false },
      ],
    });
    renderDetail();
    expect(screen.getAllByRole("alert")).toHaveLength(1);
    expect(screen.getByRole("alert")).toHaveTextContent(
      "The latest scans no longer find this agent.",
    );
    expect(screen.getByText("No longer found")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Found by 2 connections")).toBeInTheDocument();
    expect(within(sidebar()).getAllByText("Not in latest scan")).toHaveLength(2);
  });

  it("falls back to the agent's own presence when it lists no connections", () => {
    hoisted.detail = codeAgent("rules", { present: false, foundBy: [] });
    renderDetail();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "The latest scans no longer find this agent.",
    );
    expect(screen.getByText("No longer found")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Found by 1 connection")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Not in latest scan")).toBeInTheDocument();
  });

  it("shows no notice while one connection still finds the agent, and says they merged", () => {
    hoisted.detail = codeAgent("rules", {
      present: false,
      foundBy: [
        { findingId: "f1", connectionId: "c1", connectionName: "WSO2 GitHub", present: false },
        { findingId: "f9", connectionId: "c9", connectionName: "WSO2 mirror", present: true },
      ],
    });
    renderDetail();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByText("No longer found")).not.toBeInTheDocument();
    expect(within(sidebar()).getByText("Not in latest scan")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Found")).toBeInTheDocument();
    expect(
      screen.getByRole("img", {
        name: "Results from these connections were merged into one agent.",
      }),
    ).toHaveAttribute("tabindex", "0");
    expect(screen.queryByText(/It is shown once/)).not.toBeInTheDocument();
  });

  it("links an onboarded agent to the registered agent, without a banner", () => {
    hoisted.detail = detail({
      onboarded: true,
      linkedProject: "default",
      linkedAgent: "support-bot",
      onboardedAt: "2026-11-12T00:00:00Z",
    });
    renderDetail();
    const heading = screen.getByRole("heading", { name: /^Support Bot/ });
    expect(within(heading).getByText("Onboarded")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Onboarded as")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    const linked = within(sidebar()).getByRole("link", { name: "default / support-bot" });
    expect(linked).toHaveAttribute(
      "href",
      "/organizations/acme/projects/default/agents/support-bot",
    );
    expect(linked).toHaveClass("MuiLink-root");
    expect(screen.getByRole("link", { name: "Open registered agent" })).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Onboard as external agent" }),
    ).not.toBeInTheDocument();
  });

  it("shows the owner of a cloud agent, and its other connections", () => {
    hoisted.detail = detail({
      ownerHint: { email: "alice@acme.com", source: "GET /v1/agents" },
      providerCreatedAt: "2026-10-01T00:00:00Z",
      foundBy: [
        { findingId: "f1", connectionId: "c1", connectionName: "OpenAI prod", present: true },
        { findingId: "f9", connectionId: "c9", connectionName: "OpenAI audit", present: false },
      ],
    });
    renderDetail();
    expect(within(sidebar()).getByText("OpenAI audit")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Not in latest scan")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Found by 2 connections")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Created by")).toBeInTheDocument();
    expect(within(sidebar()).getByText("alice@acme.com")).toBeInTheDocument();
    expect(within(sidebar()).getByText("From GET /v1/agents")).toBeInTheDocument();
    expect(within(sidebar()).getByText("Created at the platform")).toBeInTheDocument();
  });

  it("words a cloud agent no platform lists any more, and shows possible duplicates", () => {
    hoisted.detail = detail({
      present: false,
      foundBy: [
        { findingId: "f1", connectionId: "c1", connectionName: "OpenAI prod", present: false },
      ],
      related: [
        {
          findingId: "f7",
          relation: "possible_duplicate",
          displayName: "support-bot",
          provider: "foundry",
          reason: "same name",
        },
      ],
    });
    renderDetail();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "The latest scans no longer list this agent.",
    );
    expect(within(sidebar()).getByText("Might be the same agent")).toBeInTheDocument();
    const related = within(sidebar()).getByRole("link", { name: "support-bot" });
    expect(related).toHaveAttribute("href", "/organizations/acme/agent-registry/unregistered/f7");
    expect(related).toHaveClass("MuiLink-root");
    expect(
      within(sidebar()).getByText("Possible duplicate · Microsoft Foundry · same name"),
    ).toBeInTheDocument();
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

describe("Onboarding a code agent", () => {
  const githubConnection = (over: Partial<ShadowConnectionResponse> = {}) =>
    connection({
      provider: "github",
      name: "WSO2 GitHub",
      endpoints: { api: "https://api.github.com" },
      settings: { organization: "wso2" },
      ...over,
    });

  const codeFinding = (over: Partial<ShadowFindingDetail> = {}) =>
    detail({
      provider: "github",
      kind: "code_agent",
      displayName: "Customer Support Agent",
      scope: "support-bot",
      externalId: "support-bot:samples/support/agent.py:SupportAgent",
      attributes: {
        framework: "langgraph",
        branch: "develop",
        projectPath: "/samples/support",
        language: "python",
      },
      signals: [
        {
          code: "github.d1_agent_dependency",
          name: "Agent framework dependency",
          source: "samples/support/requirements.txt",
          condition: "A dependency on an agent framework",
          observed: {},
          detectedAt: "2026-11-10T00:00:00Z",
        },
        {
          code: "github.d2_agent_import",
          name: "Agent framework import",
          source: "samples/support/app/agent.py",
          condition: "An import of an agent framework",
          observed: {},
          detectedAt: "2026-11-10T00:00:00Z",
        },
      ],
      ...over,
    });

  function ShowState() {
    const l = useLocation();
    return <pre data-testid="state">{JSON.stringify(l.state)}</pre>;
  }

  const renderDetail = (canCreateAgent = true) =>
    router(
      <FeatureManagementProvider featureAccess={{ agent_create: canCreateAgent }}>
        <Routes>
          <Route
            path="/organizations/:orgId/agent-registry/unregistered/:findingId"
            element={
              <FindingDetail
                orgName="acme"
                canManage
                listPath="/organizations/acme/agent-registry?registration=unregistered"
              />
            }
          />
          <Route
            path="/organizations/:orgId/projects/:projectId/newAgent/create/source"
            element={<ShowState />}
          />
        </Routes>
      </FeatureManagementProvider>,
      "/organizations/acme/agent-registry/unregistered/f1",
    );

  const openMenu = () => fireEvent.click(screen.getByRole("button", { name: "Onboard" }));
  const deployItem = () =>
    screen.getByRole("menuitem", { name: /^Deploy as platform-hosted agent/ });

  it("offers registering as an external agent or deploying as a platform-hosted agent", () => {
    hoisted.detail = codeFinding();
    hoisted.connection = githubConnection();
    renderDetail();
    expect(
      screen.queryByRole("link", { name: "Onboard as external agent" }),
    ).not.toBeInTheDocument();
    openMenu();
    expect(deployItem()).not.toHaveAttribute("aria-disabled", "true");
    fireEvent.click(screen.getByRole("menuitem", { name: /^Register as external agent/ }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/agent-registry/unregistered/f1/onboard",
    );
  });

  it.each([
    [
      "a GitHub Enterprise Server repository",
      {},
      githubConnection({ endpoints: { api: "https://github.acme.com/api/v3" } }),
    ],
  ])("does not deploy %s as a platform-hosted agent", (_name, over, conn) => {
    hoisted.detail = codeFinding(over);
    hoisted.connection = conn;
    renderDetail();
    openMenu();
    expect(deployItem()).toHaveAttribute("aria-disabled", "true");
    expect(
      screen.getByLabelText(
        "Only repositories on github.com can be deployed as platform-hosted agents.",
      ),
    ).toBeInTheDocument();
  });

  it("does not deploy without permission to create agents", () => {
    hoisted.detail = codeFinding();
    hoisted.connection = githubConnection();
    renderDetail(false);
    openMenu();
    expect(deployItem()).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByLabelText("You don't have permission to create agents.")).toBeVisible();
  });

  it("says so when the platform's details could not be loaded", () => {
    hoisted.detail = codeFinding();
    hoisted.connectionError = new Error("boom");
    renderDetail();
    openMenu();
    expect(deployItem()).toHaveAttribute("aria-disabled", "true");
    expect(
      screen.getByLabelText("Could not load the platform's details. Try again later."),
    ).toBeInTheDocument();
  });

  it("says so when the projects could not be loaded", () => {
    hoisted.detail = codeFinding();
    hoisted.connection = githubConnection();
    hoisted.projectsError = new Error("boom");
    renderDetail();
    openMenu();
    fireEvent.click(deployItem());
    const dialog = screen.getByRole("dialog", { name: "Deploy as platform-hosted agent" });
    expect(within(dialog).getByText("Could not load the projects.")).toBeInTheDocument();
    expect(within(dialog).queryByText(/There are no projects/)).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Continue" })).toBeDisabled();
  });

  it("asks for a project, then opens the platform-hosted agent form with a seed", () => {
    hoisted.detail = codeFinding();
    hoisted.connection = githubConnection();
    renderDetail();
    openMenu();
    fireEvent.click(deployItem());
    const dialog = screen.getByRole("dialog", { name: "Deploy as platform-hosted agent" });
    expect(within(dialog).getByRole("combobox")).toHaveTextContent("Default");
    fireEvent.click(within(dialog).getByRole("button", { name: "Continue" }));
    expect(screen.getByTestId("location")).toHaveTextContent(
      "/organizations/acme/projects/default/newAgent/create/source",
    );
    const state = JSON.parse(screen.getByTestId("state").textContent ?? "{}");
    expect(state.from).toBe("/organizations/acme/agent-registry/unregistered/f1");
    expect(state.agentSourceSeed).toEqual({
      displayName: "Customer Support Agent",
      name: "customer-support-agent",
      description: expect.stringContaining("Discovered by Shadow Agent Discovery"),
      repositoryUrl: "https://github.com/wso2/support-bot",
      branch: "develop",
      appPath: "/samples/support",
      language: "python",
      shadowFinding: { orgName: "acme", findingId: "f1" },
    });
  });

  it("falls back to the manifest's folder, then the agent file's folder, and main", () => {
    const f = codeFinding({ attributes: { framework: "langgraph" } });
    expect(seedAppPath(f)).toBe("/samples/support");
    const seed = buildAgentSourceSeed(f, githubConnection(), "acme");
    expect(seed.branch).toBe("main");
    expect(seed.language).toBe("python");
    const noManifest = codeFinding({
      attributes: {},
      signals: codeFinding().signals.filter((s) => s.code === "github.d2_agent_import"),
    });
    expect(seedAppPath(noManifest)).toBe("/samples/support/app");
    const noSignals = codeFinding({ attributes: {}, signals: [] });
    expect(seedAppPath(noSignals)).toBe("/samples/support");
    expect(seedLanguage(noSignals)).toBe("python");
    expect(seedAppPath(codeFinding({ attributes: {}, signals: [], externalId: "r:main.bal:x" })))
      .toBe("/");
    expect(seedLanguage(codeFinding({ attributes: {}, signals: [], externalId: "r:main.bal:x" })))
      .toBe("ballerina");
    expect(seedLanguage(codeFinding({ attributes: { language: "typescript" } }))).toBe("docker");
    expect(seedAppPath(codeFinding({ attributes: { projectPath: "/" } }))).toBe("/");
    expect(folderAppPath("requirements.txt")).toBe("/");
  });
});


describe("OnboardFinding", () => {
  const renderOnboard = () =>
    router(
      <Routes>
        <Route
          path="/organizations/:orgId/agent-registry/unregistered/:findingId/onboard"
          element={
            <OnboardFinding orgName="acme" listPath="/organizations/acme/agent-registry?registration=unregistered" />
          }
        />
      </Routes>,
      "/organizations/acme/agent-registry/unregistered/f1/onboard",
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
    // Once linked, it asks nothing more: the registry lists it as registered now.
    act(() => hoisted.onboard.mock.calls[0][1].onSuccess());
    expect(hoisted.addConfirmation).not.toHaveBeenCalled();
    expect(hoisted.setHidden).not.toHaveBeenCalled();
  });

  it("refuses an invalid agent name before calling anything", () => {
    hoisted.detail = detail();
    renderOnboard();
    fireEvent.change(screen.getByLabelText(/^Name/), { target: { value: "9bad" } });
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    expect(hoisted.createAgent).not.toHaveBeenCalled();
  });
});

describe("FindingDetail (listing and identifiers)", () => {
  const renderDetail = () =>
    router(
      <Routes>
        <Route
          path="/organizations/:orgId/agent-registry/unregistered/:findingId"
          element={
            <FindingDetail
              orgName="acme"
              canManage
              listPath="/organizations/acme/agent-registry?registration=unregistered"
            />
          }
        />
      </Routes>,
      "/organizations/acme/agent-registry/unregistered/f1",
    );
  const onboarded = (hidden: boolean) =>
    detail({ onboarded: true, linkedProject: "default", linkedAgent: "support-bot", hidden });

  it("has no list controls for an onboarded agent, which the registry lists as registered", () => {
    hoisted.detail = onboarded(true);
    renderDetail();
    expect(screen.queryByRole("button", { name: "Remove from list" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show in list" })).not.toBeInTheDocument();
    expect(screen.queryByText(/Removed from the Unregistered Agents list/)).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Open registered agent/ })).toBeInTheDocument();
  });

  it("shows long identifiers with a copy button, and Your inventory's labels", () => {
    hoisted.detail = detail({
      provider: "custom",
      attributes: {
        source: "LangGraph Platform",
        agentUrl: "https://agents.example.com/invoice",
        description: "Sorts invoices",
      },
    });
    renderDetail();
    expect(screen.queryByRole("button", { name: "Copy Agent URL" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Technical IDs" }));
    expect(screen.getByText("Agent URL")).toBeInTheDocument();
    expect(screen.getByText("https://agents.example.com/invoice")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy Agent URL" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy Platform ID" })).toBeInTheDocument();
    expect(screen.getByText("Runs on")).toBeInTheDocument();
    expect(screen.getByText("LangGraph Platform")).toBeInTheDocument();
    expect(screen.getByText("Sorts invoices")).toBeInTheDocument();
    expect(screen.getByText("Agent on Your inventory")).toBeInTheDocument();
    expect(screen.getByText("Listed by your inventory.")).toBeInTheDocument();
    expect(screen.queryByText("agentUrl")).not.toBeInTheDocument();
  });
});


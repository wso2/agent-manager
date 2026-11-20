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

// Shadow Agent Discovery: the agents AI platforms list, outside Agent
// Manager. These mirror the agent-manager-service OpenAPI schemas named
// Shadow*; the API calls a platform a "provider".

export type ShadowProvider =
  | "openai"
  | "anthropic"
  | "bedrock"
  | "google"
  | "foundry"
  /** The customer's own agent inventory: a REST API built to the published contract. */
  | "custom"
  /** Source repository host: scanned by the scan command on the customer's machine. */
  | "github";

/**
 * runtime: an AgentCore runtime or a Google Agent Runtime engine;
 * platform_agent: any other agent object;
 * code_agent: an agent found in a repository by the source code scan.
 */
export type ShadowFindingKind = "platform_agent" | "runtime" | "code_agent";


/** pending: a source code scan waiting for its scan command to publish results. */
export type ShadowScanStatus =
  | "pending"
  | "queued"
  | "running"
  | "succeeded"
  | "partial"
  | "failed";

export type ShadowCallOutcome =
  | "ok"
  | "skipped"
  | "auth_failed"
  | "permission_denied"
  | "not_found"
  | "rate_limited"
  | "unsupported_response"
  | "failed";

export type ShadowFieldKind = "text" | "secret" | "textarea" | "list" | "checkbox" | "key_map";

export interface ShadowEndpointSpec {
  key: string;
  label: string;
  default: string;
  placeholders: string[];
  option?: string;
  /** For an endpoint with one placeholder: the list field whose items fill it (e.g. "regions"). */
  fillFrom?: string;
  /** Items of fillFrom the endpoint is not used for. */
  fillExclude?: string[];
  /** Fixed values that fill the placeholder, instead of fillFrom. */
  fillValues?: string[];
  /** Kept behind the "Advanced configuration" button. */
  advanced?: boolean;
  /** When to change the endpoint; shown under the input. */
  help?: string;
}

export interface ShadowFieldSpec {
  key: string;
  label: string;
  kind: ShadowFieldKind;
  required: boolean;
  placeholder?: string;
  help?: string;
  defaultBool?: boolean;
  option?: string;
  /** Optional but recommended. */
  recommended?: boolean;
  /** A heading the field is shown under, for example "Services to scan". */
  section?: string;
  /** An optional setting kept behind the "Advanced configuration" button. */
  advanced?: boolean;
}

/** At least one of these fields must hold a credential. */
export interface ShadowCredentialChoice {
  fields: string[];
  message: string;
}

/** One thing a scan lists, and the fields or options it needs. */
export interface ShadowCapabilitySpec {
  key: string;
  title: string;
  description: string;
  signals: string[];
  needs: string[];
}

export interface ShadowPermissionSpec {
  title: string;
  /** In one or two plain sentences, what Agent Manager does with this credential or source. */
  summary?: string;
  /** What Agent Manager reads with it. */
  reads?: string[];
  /** How to create it and the permissions it needs. */
  items: string[];
  policy?: string;
  note?: string;
}

export interface ShadowProviderSpec {
  provider: ShadowProvider;
  displayName: string;
  description: string;
  endpoints: ShadowEndpointSpec[];
  fields: ShadowFieldSpec[];
  permissions: ShadowPermissionSpec[];
  docs: string[];
  credentialChoice?: ShadowCredentialChoice;
  /**
   * A source repository host (GitHub): no credential is stored, and
   * its repositories are scanned by the scan command on the customer's machine.
   */
  sourceHost?: boolean;
  capabilities: ShadowCapabilitySpec[];
  /** What a scan of the platform never reads or lists. */
  notListed?: string[];
  /** The steps to make the platform listable (Custom platform). */
  setupSteps?: string[];
  /** The OpenAPI document (YAML) the customer implements (Custom platform). */
  apiContract?: string;
}

/** One kind of evidence that a platform lists an agent. */
export interface ShadowSignalDefinition {
  code: string;
  provider: string;
  name: string;
  /** The listing call and the exact condition, in the platform's field names. */
  condition: string;
}

export interface ShadowProviderListResponse {
  providers: ShadowProviderSpec[];
  signals: ShadowSignalDefinition[];
}

export type ShadowSettingValue = string | boolean | string[] | Record<string, string>;

export interface ShadowConnectionRequest {
  name: string;
  provider: ShadowProvider;
  endpoints?: Record<string, string>;
  settings: Record<string, ShadowSettingValue>;
  /** On update: optional secret fields whose stored value is removed. */
  removeSecrets?: string[];
}

export interface ShadowConnectionCheck {
  name: string;
  outcome: ShadowCallOutcome | string;
  message?: string;
  unlocks?: string[];
}

export interface ShadowConnectionTest {
  ok: boolean;
  checks: ShadowConnectionCheck[];
}

export interface ShadowConnectionResponse {
  id: string;
  name: string;
  provider: ShadowProvider;
  endpoints: Record<string, string>;
  settings: Record<string, unknown>;
  secretHints: Record<string, string>;
  endpointWarnings: string[];
  lastTestedAt?: string;
  lastTest?: ShadowConnectionTest;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface ShadowConnectionListResponse {
  connections: ShadowConnectionResponse[];
}

export interface ShadowScanCallResult {
  call: string;
  outcome: ShadowCallOutcome;
  message?: string;
  items: number;
}

export interface ShadowScanConnectionResult {
  connectionId: string;
  connectionName: string;
  provider: string;
  outcome: string;
  message?: string;
  findingsCount: number;
  calls: ShadowScanCallResult[];
  /** Repositories a source code scan scanned. Only on GET a scan, never the list. */
  scannedRepositories?: string[];
  /** Repositories a source code scan could not scan. Only on GET a scan, never the list. */
  failedRepositories?: ShadowScanFailedRepository[];
}

/** A repository a source code scan could not scan, and why. */
export interface ShadowScanFailedRepository {
  name: string;
  message: string;
}

/** Generate command for a GitHub connection. */
export interface ShadowRunnerScanRequest {
  /** Send matches the rules cannot prove to the customer's LLM for review. */
  aiReview: boolean;
  /** A second model must agree with the reviewer; needs aiReview. */
  llmJudge: boolean;
}

/**
 * The pending scan and the parts of the command that runs it; the console
 * composes the command. The token is shown once and never cached.
 */
export interface ShadowRunnerScanResponse {
  scanId: string;
  /** The scan token the command publishes results with. */
  token: string;
  /** When the scan token expires (1 hour). */
  expiresAt: string;
  /** The Agent Manager address the scanner publishes to. */
  ampUrl: string;
  /** The scanner container image. */
  image: string;
}

export interface ShadowScanRequest {
  /** The connections to scan; every connection when empty. */
  connectionIds?: string[];
}

export interface ShadowScanResponse {
  id: string;
  status: ShadowScanStatus;
  connectionIds: string[];
  results: ShadowScanConnectionResult[];
  findingsCount: number;
  requestedBy: string;
  startedAt?: string;
  finishedAt?: string;
  createdAt: string;
}

export interface ShadowScanListResponse {
  scans: ShadowScanResponse[];
}

/** DELETE .../scans: how many finished scans were deleted. */
export interface ShadowScanClearResponse {
  deleted: number;
}

/** DELETE .../findings: how many agents were deleted. */
export interface ShadowFindingClearResponse {
  deleted: number;
}

export interface ShadowAgentSummary {
  /** The agents found that their platforms still list, each agent once. */
  totalFindings: number;
  byProvider: Record<string, number>;
  /** How many of those agents are onboarded as external agents. */
  onboarded: number;
  connections: number;
  latestScan?: ShadowScanResponse;
}

export interface ShadowOwnerHint {
  name?: string;
  email?: string;
  username?: string;
  type?: string;
  source?: string;
  createdAt?: string;
}

/** One connection that found an agent: the agent's finding in that connection. */
export interface ShadowFindingSource {
  findingId: string;
  connectionId: string;
  connectionName: string;
  present: boolean;
}

/**
 * One agent a platform lists, shown once however many connections found it:
 * its finding in the connection that saw it last, and every connection in foundBy.
 */
export interface ShadowFindingSummary {
  id: string;
  connectionId: string;
  connectionName: string;
  provider: ShadowProvider;
  kind: ShadowFindingKind;
  /** The platform's own ID of the agent (agent ID, ARN or resource name). */
  externalId: string;
  displayName: string;
  /** Where the agent lives at the platform (project, workspace, region or location). */
  scope: string;
  /** The agent's state as its platform reports it. */
  state: string;
  /** Who created the agent, only when its platform's listing says so. */
  ownerHint: ShadowOwnerHint;
  providerCreatedAt?: string;
  firstSeenAt: string;
  lastSeenAt: string;
  /** False once a complete listing of its connection no longer returns the agent. */
  present: boolean;
  foundBy: ShadowFindingSource[];
  /** Whether the agent is onboarded as an external agent. */
  onboarded: boolean;
  /** The project and name of the external agent it was onboarded as. */
  linkedProject?: string;
  linkedAgent?: string;
  onboardedAt?: string;
  /** The onboarded agent was removed from the Unregistered Agents list. */
  hidden: boolean;
}

export interface ShadowFindingListResponse {
  findings: ShadowFindingSummary[];
  total: number;
  limit: number;
  offset: number;
}

export interface ShadowFindingListQuery {
  /** A platform. */
  provider?: string;
  /** "true": only onboarded agents; "false": only the others. */
  onboarded?: "true" | "false";
  connectionId?: string;
  /** Keep the agents this scan found; without it, only the agents still listed. */
  scanId?: string;
  search?: string;
  limit?: number;
  offset?: number;
}

/** Evidence that a platform lists the agent. */
export interface ShadowFindingSignal {
  code: string;
  name: string;
  source: string;
  condition: string;
  observed: Record<string, unknown>;
  detectedAt: string;
}

export interface ShadowFindingRelation {
  findingId: string;
  relation: string;
  reason?: string;
  displayName?: string;
  provider?: string;
}

export interface ShadowFindingDetail extends ShadowFindingSummary {
  /** What the platform's agent listing reports (allow-listed fields only). */
  attributes: Record<string, unknown>;
  /** Agents on other platforms that may be the same agent. */
  related: ShadowFindingRelation[];
  signals: ShadowFindingSignal[];
  /** Who onboarded the agent. */
  onboardedBy?: string;
}

/** PUT .../findings/{findingId}/hidden: remove an onboarded agent from the list, or put it back. */
export interface ShadowFindingHiddenRequest {
  hidden: boolean;
}

/** POST .../findings/{findingId}/onboarding: the registered external agent. */
export interface ShadowOnboardingRequest {
  projectName: string;
  agentName: string;
}

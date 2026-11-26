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

import type {
  AgentRegistryApiAgent,
  AgentRegistryApiEnvironment,
  AgentRegistryApiListResponse,
  AgentRegistryEntry,
  AgentRegistryEntryDetail,
  AgentRegistryEnvironmentDetail,
  AgentRegistryListResponse,
  AgentResponse,
  GetAgentRegistryEntryPathParams,
  ListAgentRegistryPathParams,
  ShadowFindingSummary,
  ShadowProviderListResponse,
} from "@agent-management-platform/types";
import { httpGET, SERVICE_BASE } from "../utils";
import { getProject } from "./projects";
import { listShadowFindings, listShadowProviders } from "./shadowAgents";

// "createdBy" stands in for the owner: owners do not exist yet.

/** A managed agent's row, from its AgentSummary and registry record. */
function managedEntry(agent: AgentRegistryApiAgent): AgentRegistryEntry {
  const record = agent.registry;
  return {
    name: agent.name,
    displayName: agent.displayName || agent.name,
    description: record.description,
    management: "managed",
    projectName: agent.projectName,
    projectDisplayName: agent.projectDisplayName || agent.projectName,
    hosting: record.hosting,
    interface: record.interface ?? null,
    kindName: record.kindName,
    kindVersion: record.kindVersion,
    // A creator whose user could not be looked up keeps only the id, as on the agent page.
    createdBy: record.createdBy
      ? {
          id: record.createdBy.id,
          displayName: record.createdBy.displayName || record.createdBy.id,
        }
      : null,
    labels: record.labels,
    environments: record.environments.map((env) => ({
      name: env.name,
      displayName: env.displayName,
      isProduction: env.isProduction,
      deploymentStatus: env.deploymentStatus,
      lastDeployedAt: env.lastDeployedAt ?? null,
      llmProviders: env.llmProviders,
      mcpServers: env.mcpServers,
    })),
    createdAt: record.createdAt,
  };
}

/** Every managed agent in the organization, from every project, with its registry record. */
export async function listAgentRegistry(
  params: ListAgentRegistryPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentRegistryListResponse> {
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${SERVICE_BASE}/orgs/${encodeURIComponent(params.orgName ?? "")}/agents`, {
    searchParams: { view: "registry" },
    token,
  });
  const body: AgentRegistryApiListResponse = await res.json();
  return { agents: body.agents.map(managedEntry) };
}

/** The most findings the findings API returns in one page. */
const FINDINGS_PAGE_SIZE = 200;

/** An unmanaged agent's row, from its Shadow Agent Discovery finding. */
function unmanagedEntry(
  finding: ShadowFindingSummary,
  providerNames: Map<string, string>,
): AgentRegistryEntry {
  return {
    name: finding.id,
    displayName: finding.displayName,
    management: "unmanaged",
    discovery: {
      findingId: finding.id,
      provider: finding.provider,
      providerDisplayName: providerNames.get(finding.provider) ?? finding.provider,
      scope: finding.scope,
      ownerHint: {
        name: finding.ownerHint.name,
        email: finding.ownerHint.email,
        source: finding.ownerHint.source,
      },
      firstSeenAt: finding.firstSeenAt,
      lastSeenAt: finding.lastSeenAt,
      present: finding.present,
      onboarded: finding.onboarded,
      connectionName: finding.connectionName,
      kind: finding.kind,
      externalId: finding.externalId,
      state: finding.state,
      providerCreatedAt: finding.providerCreatedAt,
      foundBy: finding.foundBy,
    },
    projectName: "",
    projectDisplayName: "",
    hosting: "external",
    interface: null,
    createdBy: null,
    labels: {},
    environments: [],
    createdAt: finding.firstSeenAt,
  };
}

/**
 * Every unmanaged agent: the Shadow Agent Discovery findings that are not onboarded.
 * Without a scan ID the findings API returns only the agents their platforms still list.
 */
export async function listUnmanagedAgents(
  params: ListAgentRegistryPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentRegistryListResponse> {
  const providersRequest = listShadowProviders(params.orgName, getToken);
  const findings: ShadowFindingSummary[] = [];
  for (let offset = 0; ; offset += FINDINGS_PAGE_SIZE) {
    const page = await listShadowFindings(
      params.orgName,
      { onboarded: "false", limit: FINDINGS_PAGE_SIZE, offset },
      getToken,
    );
    findings.push(...page.findings);
    if (page.findings.length === 0 || findings.length >= page.total) break;
  }
  const providerNames = providerNamesOf(await providersRequest);
  return { agents: findings.map((finding) => unmanagedEntry(finding, providerNames)) };
}

/** The display name of each Shadow Agent Discovery platform. */
function providerNamesOf(response: ShadowProviderListResponse): Map<string, string> {
  return new Map(response.providers.map((p) => [p.provider as string, p.displayName]));
}

/** True when a request failed because the resource does not exist. */
function isNotFound(error: unknown): boolean {
  return (error as { status?: number } | null)?.status === 404;
}

/** One environment on the agent overview: the list's fields and the agent's governance there. */
function environmentDetail(env: AgentRegistryApiEnvironment): AgentRegistryEnvironmentDetail {
  const governance = env.governance;
  return {
    name: env.name,
    displayName: env.displayName,
    isProduction: env.isProduction,
    deploymentStatus: env.deploymentStatus,
    lastDeployedAt: env.lastDeployedAt ?? null,
    llmProviders: env.llmProviders,
    mcpServers: env.mcpServers.map((server) => ({ ...server, tools: server.tools ?? [] })),
    agentIdStatus: governance?.agentIdStatus ?? null,
    llmConfigurations: governance?.llmConfigurations ?? [],
    monitors: governance?.monitors ?? [],
    inboundSecurity: governance?.inboundSecurity ?? null,
    a2aCard: governance?.a2aSkills ? { skills: governance.a2aSkills } : undefined,
  };
}

/**
 * One managed agent's registry record, with its governance in each environment, or null
 * when there is no such agent. Reads GET agent with view=registry, and the project for its name.
 */
export async function getAgentRegistryEntry(
  params: GetAgentRegistryEntryPathParams,
  getToken?: () => Promise<string>,
): Promise<AgentRegistryEntryDetail | null> {
  const { orgName = "", projName = "", agentName = "" } = params;
  const token = getToken ? await getToken() : undefined;
  const agentRequest = httpGET(
    `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/projects/${encodeURIComponent(projName)}` +
      `/agents/${encodeURIComponent(agentName)}`,
    { searchParams: { view: "registry" }, token },
  );
  const projectRequest = getProject({ orgName, projName }, getToken);
  let agent: AgentResponse & { registry?: AgentRegistryApiAgent["registry"] };
  try {
    agent = await (await agentRequest).json();
  } catch (error) {
    projectRequest.catch(() => undefined);
    if (isNotFound(error)) return null;
    throw error;
  }
  const project = await projectRequest;
  if (!agent.registry) return null;
  const entry = managedEntry({
    name: agent.name,
    displayName: agent.displayName || agent.name,
    projectName: projName,
    projectDisplayName: project.displayName || projName,
    registry: agent.registry,
  });
  return { ...entry, environments: agent.registry.environments.map(environmentDetail) };
}

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

import {
  Plane,
  Hotel,
  Headset,
  ShieldCheck,
  type LucideIcon,
} from "@wso2/oxygen-ui-icons-react";
import { getScriptRef } from "@agent-management-platform/shared-component";

export interface SampleAgentEnvVar {
  key: string;
  // A concrete default (e.g. a required literal, not a secret) to pre-fill.
  // Left undefined for values the user must supply themselves (API keys, URLs).
  value?: string;
  isSensitive?: boolean;
}

// Static catalog of ready-made agents shown on the "Start Quickly with a
// Sample" card. Selecting one seeds the Platform-Hosted Agent form's
// repository/branch/app path/env fields so the user only has to fill in
// secret values before deploying.
//
// Sourced from each sample's own README under samples/<id> in this repo
// (the "Configure Agent Details" / "Environment Variables" sections) — keep
// this list in sync with those READMEs when a sample's deployment steps change.
export interface SampleAgentDefinition {
  id: string;
  title: string;
  description: string;
  icon: LucideIcon;
  // Short display names of the third-party services/APIs this sample talks to
  // (e.g. "Tavily", not "TAVILY_API_KEY") — shown to the user up front as tags,
  // distinct from the raw env var keys below.
  thirdPartyServices: string[];
  repositoryUrl: string;
  branch: string;
  appPath: string;
  language: string;
  languageVersion: string;
  runCommand: string;
  envVars: SampleAgentEnvVar[];
}

const AGENT_MANAGER_REPO = "https://github.com/wso2/agent-manager";

// Pin samples to this console's release tag (amp/vX.Y.Z) so a released console
// deploys the samples it shipped with; dev builds without ampVersion use main.
const SAMPLES_REF = getScriptRef();

// Shared by every sample below except where a sample's own README documents
// a different Start Command.
const PYTHON_SAMPLE_DEFAULTS = {
  repositoryUrl: AGENT_MANAGER_REPO,
  branch: SAMPLES_REF,
  language: "python",
  languageVersion: "3.11",
  runCommand: "python main.py",
} as const;

export const sampleAgents: SampleAgentDefinition[] = [
  {
    ...PYTHON_SAMPLE_DEFAULTS,
    id: "customer-support-agent",
    title: "Support Agent",
    description: "AI-powered customer support agent for travel services",
    icon: Plane,
    thirdPartyServices: ["OpenAI", "Tavily", "PostgreSQL"],
    appPath: "/samples/customer-support-agent",
    envVars: [
      { key: "OPENAI_API_KEY", isSensitive: true },
      { key: "TAVILY_API_KEY", isSensitive: true },
      { key: "DATABASE_URL", isSensitive: true },
    ],
  },
  {
    ...PYTHON_SAMPLE_DEFAULTS,
    id: "hotel-booking-agent",
    title: "Hotel Booking Agent",
    description: "AI-powered hotel booking assistant",
    icon: Hotel,
    thirdPartyServices: ["OpenAI", "Pinecone (Hotel API)"],
    appPath: "/samples/hotel-booking-agent/agent",
    runCommand: "python -m uvicorn app:app --host 0.0.0.0 --port 8000",
    envVars: [
      { key: "OPENAI_API_KEY", isSensitive: true },
      { key: "HOTEL_API_BASE_URL" },
    ],
  },
  {
    ...PYTHON_SAMPLE_DEFAULTS,
    id: "it-helpdesk-agent",
    title: "IT Helpdesk Agent",
    description: "AI-powered IT helpdesk agent for employee technical support",
    icon: Headset,
    thirdPartyServices: ["OpenAI"],
    appPath: "/samples/it-helpdesk-agent",
    envVars: [
      { key: "OPENAI_API_KEY", isSensitive: true },
    ],
  },
  {
    ...PYTHON_SAMPLE_DEFAULTS,
    id: "insurance-support-agent",
    title: "Insurance Support Agent",
    description: "Customer support agent for policies and claims",
    icon: ShieldCheck,
    thirdPartyServices: ["OpenAI"],
    appPath: "/samples/insurance-support-agent/agent",
    envVars: [
      { key: "OPENAI_API_KEY", isSensitive: true },
      { key: "PORT", value: "8000" },
    ],
  },
];

export const getSampleAgentById = (id: string | null): SampleAgentDefinition | undefined =>
  sampleAgents.find((sample) => sample.id === id);

// Every sample's README lives at this same path relative to the repo root
// (samples/<id>/README.md), regardless of where its appPath points within
// the sample's own folder — see samples/*/README.md in this repo.
export const getSampleReadmeUrl = (sample: SampleAgentDefinition): string =>
  `${AGENT_MANAGER_REPO}/blob/${SAMPLES_REF}/samples/${sample.id}/README.md`;

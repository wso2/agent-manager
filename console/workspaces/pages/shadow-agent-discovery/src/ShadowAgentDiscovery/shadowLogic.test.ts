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

import { describe, expect, it } from "vitest";
import type { ShadowProviderSpec } from "@agent-management-platform/types";
import {
  credentialChoiceError,
  scopedKeyCount,
  evaluateCapabilities,
  hasValue,
  optionActive,
} from "./capabilities";
import {
  AGENT_PLATFORMS,
  formatCount,
  itemCount,
  kindLabel,
  ownerLabel,
  PROVIDER_LABELS,
  SOURCE_HOSTS,
  STATUS_LABELS,
  unlockLabels,
} from "./shadowUi";
import {
  endpointVisible,
  fieldVisible,
  initialDraft,
  toSettings,
  validateDraft,
} from "./subComponents/ConnectionEditor";
import { getSourceRepoScanData } from "../sourceRepoScan/dummyData";

// The shape of the service's catalog (catalog.go, capabilities.go), cut down.
const openai: ShadowProviderSpec = {
  provider: "openai",
  displayName: "OpenAI",
  description: "",
  endpoints: [
    { key: "api", label: "OpenAI API", default: "https://api.openai.com/v1", placeholders: [] },
  ],
  fields: [{ key: "scopedKeys", label: "Project keys", kind: "key_map", required: true }],
  permissions: [],
  docs: [],
  capabilities: [
    {
      key: "openai.agents",
      title: "Agents",
      description: "",
      signals: ["openai.agent_object"],
      needs: ["scopedKeys"],
    },
  ],
};

const foundry: ShadowProviderSpec = {
  provider: "foundry",
  displayName: "Microsoft Foundry Agent Service",
  description: "",
  endpoints: [
    {
      key: "login",
      label: "Microsoft Entra login",
      default: "https://login.microsoftonline.com",
      placeholders: [],
      advanced: true,
    },
  ],
  fields: [
    { key: "tenantId", label: "Directory (tenant) ID", kind: "text", required: true },
    { key: "clientSecret", label: "Client secret", kind: "secret", required: true },
    {
      key: "allSubscriptions",
      label: "Scan every subscription the app can read",
      kind: "checkbox",
      required: false,
      defaultBool: true,
    },
    {
      key: "subscriptionIds",
      label: "Subscription IDs",
      kind: "list",
      required: true,
      option: "!allSubscriptions",
    },
    {
      key: "includeClassicAgents",
      label: "Also list classic agents (retires 31 March 2027)",
      kind: "checkbox",
      required: false,
      defaultBool: true,
    },
  ],
  permissions: [],
  docs: [],
  capabilities: [
    { key: "foundry.agents", title: "Foundry agents", description: "", signals: [], needs: [] },
    {
      key: "foundry.classic",
      title: "Classic agents",
      description: "",
      signals: [],
      needs: ["includeClassicAgents"],
    },
  ],
};

const bedrock: ShadowProviderSpec = {
  provider: "bedrock",
  displayName: "Amazon Bedrock",
  description: "",
  credentialChoice: {
    fields: ["scanAgentCore", "scanAgentRegistry", "scanClassicAgents"],
    message: "Select at least one Bedrock service to scan.",
  },
  endpoints: [
    {
      key: "sts",
      label: "AWS STS",
      default: "https://sts.{region}.amazonaws.com",
      placeholders: ["region"],
      option: "roleArn",
    },
    {
      key: "agentcoreControl",
      label: "AgentCore control plane",
      default: "https://bedrock-agentcore-control.{region}.amazonaws.com",
      placeholders: ["region"],
      option: "scanAgentCore",
    },
    {
      key: "agentRegistry",
      label: "AWS Agent Registry",
      default: "https://agent-registry-control.{region}.api.aws",
      placeholders: ["region"],
      option: "scanAgentRegistry",
    },
    {
      key: "bedrockAgent",
      label: "Bedrock Agents",
      default: "https://bedrock-agent.{region}.amazonaws.com",
      placeholders: ["region"],
      option: "scanClassicAgents",
    },
  ],
  fields: [
    { key: "roleArn", label: "Role to assume", kind: "text", required: false, advanced: true },
    ...["scanAgentCore", "scanAgentRegistry", "scanClassicAgents"].map((key) => ({
      key,
      label: key,
      kind: "checkbox" as const,
      required: false,
      defaultBool: true,
      section: "Services to scan",
    })),
  ],
  permissions: [],
  docs: [],
  capabilities: [
    { key: "bedrock.agentcore", title: "", description: "", signals: [], needs: ["scanAgentCore"] },
    {
      key: "bedrock.registry",
      title: "",
      description: "",
      signals: [],
      needs: ["scanAgentRegistry"],
    },
    {
      key: "bedrock.classic",
      title: "",
      description: "",
      signals: [],
      needs: ["scanClassicAgents"],
    },
  ],
};

const stored = {
  id: "1",
  name: "x",
  provider: "openai" as const,
  endpoints: {},
  settings: {},
  secretHints: { "scopedKeys.proj_a": "sk-p…aaaa" } as Record<string, string>,
  endpointWarnings: [],
  createdBy: "",
  createdAt: "",
  updatedAt: "",
};

describe("labels", () => {
  it("picks the most useful owner line", () => {
    expect(ownerLabel({ name: "Alice", email: "alice@acme.com" })).toBe("alice@acme.com");
    expect(ownerLabel({ username: "arn:aws:iam::1:user/bob" })).toBe("arn:aws:iam::1:user/bob");
    expect(ownerLabel(undefined)).toBe("");
    expect(formatCount(undefined)).toBe("0");
  });

  it("counts items in the singular and plural", () => {
    expect(itemCount(1)).toBe("1 item");
    expect(itemCount(0)).toBe("0 items");
    expect(itemCount(2500)).toBe(`${(2500).toLocaleString()} items`);
  });

  it("names the evidence a check lists and keeps anything else as given", () => {
    const names = new Map([["openai.agent_object", "Agent object"]]);
    expect(
      unlockLabels(["openai.agent_object", "Something else", "openai.agent_object"], names),
    ).toEqual(["Agent object", "Something else"]);
  });

  it("names each kind of agent in its platform's words", () => {
    expect(kindLabel("platform_agent", "openai")).toBe("Agent");
    expect(kindLabel("platform_agent", "anthropic")).toBe("Managed Agent");
    expect(kindLabel("runtime", "bedrock")).toBe("AgentCore runtime");
    expect(kindLabel("platform_agent", "bedrock")).toBe("Bedrock agent");
    expect(kindLabel("runtime", "google")).toBe("Agent Runtime engine");
    expect(kindLabel("platform_agent", "google")).toBe("Agent Registry agent");
    expect(kindLabel("platform_agent", "foundry")).toBe("Foundry agent");
    expect(kindLabel("runtime")).toBe("Agent runtime");
    expect(kindLabel("something")).toBe("something");
  });

  it("has only the review statuses, and the five agent platforms", () => {
    expect(STATUS_LABELS).toEqual({ new: "New", reviewed: "Reviewed", dismissed: "Dismissed" });
    expect(AGENT_PLATFORMS).toEqual(["openai", "anthropic", "bedrock", "google", "foundry"]);
    for (const host of SOURCE_HOSTS) expect(AGENT_PLATFORMS).not.toContain(host);
    expect(Object.keys(PROVIDER_LABELS).sort()).toEqual(
      [...AGENT_PLATFORMS, ...SOURCE_HOSTS].sort(),
    );
  });
});

describe("what Agent Manager lists", () => {
  const available = (d: ReturnType<typeof initialDraft>, existing?: typeof stored) =>
    evaluateCapabilities(openai, d, existing)
      .filter((c) => c.available)
      .map((c) => c.capability.key);

  it("lists agents with project keys, and nothing else", () => {
    const d = initialDraft(openai, undefined);
    expect(available(d)).toEqual([]);
    d.keyMaps.scopedKeys = [{ id: "proj_a", key: "sk-proj" }];
    expect(available(d)).toEqual(["openai.agents"]);
  });

  it("names what is missing", () => {
    const d = initialDraft(openai, undefined);
    expect(evaluateCapabilities(openai, d, undefined)[0]).toMatchObject({
      available: false,
      missing: ["Project keys"],
    });
  });

  it("checks a key map row's stored key and ignores fields of an unticked option", () => {
    const d = initialDraft(openai, undefined);
    d.keyMaps.scopedKeys = [{ id: "proj_a", key: "" }];
    expect(hasValue(openai, d, undefined, "scopedKeys")).toBe(false);
    const withKey = { ...stored, secretHints: { "scopedKeys.proj_a": "sk-p…aaaa" } };
    expect(hasValue(openai, d, withKey, "scopedKeys")).toBe(true);
    const f = initialDraft(foundry, undefined);
    f.values.subscriptionIds = "sub-1";
    expect(hasValue(foundry, f, undefined, "subscriptionIds")).toBe(false);
    f.values.allSubscriptions = false;
    expect(hasValue(foundry, f, undefined, "subscriptionIds")).toBe(true);
    expect(hasValue(openai, d, undefined, "nope")).toBe(false);
  });

  it("drops classic agents when they are unticked", () => {
    const f = initialDraft(foundry, undefined);
    const keys = () =>
      evaluateCapabilities(foundry, f, undefined)
        .filter((c) => c.available)
        .map((c) => c.capability.key);
    expect(keys()).toEqual(["foundry.agents", "foundry.classic"]);
    f.values.includeClassicAgents = false;
    expect(keys()).toEqual(["foundry.agents"]);
  });

  it("counts the project or workspace keys of OpenAI and Anthropic platforms only", () => {
    expect(scopedKeyCount("openai", { "scopedKeys.p": "x", apiKey: "sk-a…" })).toBe(1);
    expect(scopedKeyCount("anthropic", {})).toBe(0);
    expect(scopedKeyCount("bedrock", {})).toBeUndefined();
    expect(credentialChoiceError(openai, initialDraft(openai, undefined), undefined)).toBe(
      undefined,
    );
  });

  it("reads checkbox options, negated with !", () => {
    expect(optionActive(undefined, {})).toBe(true);
    expect(optionActive("a", { a: true })).toBe(true);
    expect(optionActive("a", { a: false })).toBe(false);
    expect(optionActive("!a", { a: false })).toBe(true);
    expect(optionActive("!a", { a: true })).toBe(false);
  });
});

describe("services to scan (Amazon Bedrock)", () => {
  it("shows a service's endpoint only when it is ticked, and STS only with a role", () => {
    const d = initialDraft(bedrock, undefined);
    const shown = () =>
      bedrock.endpoints
        .filter((e) => endpointVisible(bedrock, d, undefined, e.option))
        .map((e) => e.key);
    expect(shown()).toEqual(["agentcoreControl", "agentRegistry", "bedrockAgent"]);
    d.values.roleArn = "arn:aws:iam::123456789012:role/AmpShadowDiscovery";
    expect(shown()).toEqual(["sts", "agentcoreControl", "agentRegistry", "bedrockAgent"]);
    d.values.scanAgentRegistry = false;
    expect(shown()).toEqual(["sts", "agentcoreControl", "bedrockAgent"]);
  });

  it("drops a service's listing when it is unticked, and needs one service", () => {
    const d = initialDraft(bedrock, undefined);
    d.name = "x";
    const keys = () =>
      evaluateCapabilities(bedrock, d, undefined)
        .filter((c) => c.available)
        .map((c) => c.capability.key);
    expect(keys()).toEqual(["bedrock.agentcore", "bedrock.registry", "bedrock.classic"]);
    d.values.scanAgentCore = false;
    d.values.scanAgentRegistry = false;
    expect(keys()).toEqual(["bedrock.classic"]);
    expect(validateDraft(bedrock, d, undefined).credentialChoice).toBeUndefined();
    d.values.scanClassicAgents = false;
    expect(validateDraft(bedrock, d, undefined).credentialChoice).toBe(
      "Select at least one Bedrock service to scan.",
    );
  });
});

describe("platform form", () => {
  it("prefills official endpoints and checkbox defaults", () => {
    const d = initialDraft(foundry, undefined);
    expect(d.endpoints.login).toBe("https://login.microsoftonline.com");
    expect(d.values.allSubscriptions).toBe(true);
    expect(d.values.includeClassicAgents).toBe(true);
    expect(fieldVisible(foundry.fields[3], d)).toBe(false);
    d.values.allSubscriptions = false;
    expect(fieldVisible(foundry.fields[3], d)).toBe(true);
  });

  it("requires a field only while it is shown", () => {
    const d = initialDraft(foundry, undefined);
    d.name = "Foundry";
    d.values.tenantId = "t";
    d.values.clientSecret = "s";
    expect(validateDraft(foundry, d, undefined).subscriptionIds).toBeUndefined();
    d.values.allSubscriptions = false;
    expect(validateDraft(foundry, d, undefined).subscriptionIds).toBe(
      "Subscription IDs needs at least one value",
    );
  });

  it("requires at least one project key", () => {
    const d = initialDraft(openai, undefined);
    d.name = "x";
    d.keyMaps.scopedKeys = [];
    expect(validateDraft(openai, d, undefined).scopedKeys).toBe(
      "Project keys needs at least one ID and key",
    );
    d.keyMaps.scopedKeys = [{ id: "proj_a", key: "sk-proj-a" }];
    expect(validateDraft(openai, d, undefined).scopedKeys).toBeUndefined();
  });

  it("asks for the key of a new project key row, and the ID of a key", () => {
    const d = initialDraft(openai, undefined);
    d.name = "x";
    d.keyMaps.scopedKeys = [{ id: "proj_new", key: "" }];
    expect(validateDraft(openai, d, undefined).scopedKeys).toBe("Enter the key for proj_new");
    d.keyMaps.scopedKeys = [{ id: " ", key: "sk" }];
    expect(validateDraft(openai, d, undefined).scopedKeys).toBe(
      "Enter the ID for each key in Project keys",
    );
    // A stored key for the ID is kept.
    const withKey = { ...stored, secretHints: { "scopedKeys.proj_new": "sk-p…aaaa" } };
    d.keyMaps.scopedKeys = [{ id: "proj_new", key: "" }];
    expect(validateDraft(openai, d, withKey).scopedKeys).toBeUndefined();
  });

  it("rejects endpoints with a query", () => {
    const d = initialDraft(openai, undefined);
    d.name = "x";
    d.keyMaps.scopedKeys = [{ id: "proj_a", key: "k" }];
    d.endpoints.api = "https://api.openai.com/v1?key=1";
    expect(validateDraft(openai, d, undefined)["endpoint:api"]).toBeDefined();
  });

  it("never sends an empty secret, and sends lists, maps and checkboxes typed", () => {
    const d = initialDraft(foundry, undefined);
    d.values.allSubscriptions = false;
    d.values.subscriptionIds = "sub-1\n sub-2 ,";
    const s = toSettings(foundry, d);
    expect(s).not.toHaveProperty("clientSecret");
    expect(s.subscriptionIds).toEqual(["sub-1", "sub-2"]);
    expect(s.allSubscriptions).toBe(false);
    expect(s.includeClassicAgents).toBe(true);
    const o = initialDraft(openai, undefined);
    o.keyMaps.scopedKeys = [
      { id: "proj_a", key: "" },
      { id: " ", key: "x" },
    ];
    expect(toSettings(openai, o).scopedKeys).toEqual({ proj_a: "" });
  });
});

describe("source repository preview data", () => {
  it("is consistent with the design's detector weights and verdict rule", () => {
    const data = getSourceRepoScanData();
    const weight = new Map(data.detectors.map((d) => [d.id, d]));
    for (const s of data.suspects) {
      const ids = [...new Set(s.hits.map((h) => h.detectorId))];
      const score = ids.reduce((n, id) => n + (weight.get(id)?.weight ?? 0), 0);
      expect(s.score).toBe(score);
      const strong = ids.some((id) => weight.get(id)?.tier === "High" || id === "D7");
      const verdict =
        score >= 60 && strong ? "Agent" : score >= 30 && strong ? "Possible agent" : "LLM consumer";
      expect(s.verdict).toBe(verdict);
    }
  });
});

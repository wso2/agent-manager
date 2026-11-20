import { describe, it, expect } from "vitest";
import { buildModelConfig, buildMCPConfig, findLowestEnvironmentName, buildCatalogAgentPayload, deriveCatalogEnvSeed } from "./buildAgentPayload";
import { mcpEntryVarNames } from "./mcpEnvVarNames";
import type { CreateAgentFormValues, LLMProviderFormEntry, MCPProxyFormEntry } from "./schema";
import type { AgentKindConfigSchemaItem, OrgProjPathParams } from "@agent-management-platform/types";

function entry(over: Partial<LLMProviderFormEntry> = {}): LLMProviderFormEntry {
  return {
    selectedProviderByEnv: { Development: { uuid: "u1", handle: "openai" } },
    urlVarName: undefined,
    apikeyVarName: undefined,
    guardrails: [],
    ...over,
  };
}

describe("buildModelConfig (flat shape)", () => {
  it("maps a single provider with no env overrides", () => {
    const out = buildModelConfig([entry()], "Development");
    expect(out).toEqual([{ providerName: "openai" }]);
  });

  it("uses the provider selected for the lowest environment", () => {
    const out = buildModelConfig([
      entry({
        selectedProviderByEnv: {
          Production: { uuid: "u2", handle: "anthropic" },
          Development: { uuid: "u1", handle: "openai" },
        },
      }),
    ], "Development");

    expect(out).toEqual([{ providerName: "openai" }]);
  });

  it("includes env-var name overrides", () => {
    const out = buildModelConfig([entry({ urlVarName: "MY_URL", apikeyVarName: "MY_KEY" })], "Development");
    expect(out?.[0]).toMatchObject({
      providerName: "openai",
      environmentVariables: [
        { key: "url", name: "MY_URL" },
        { key: "apikey", name: "MY_KEY" },
      ],
    });
  });

  it("preserves guardrail policies in configuration", () => {
    const out = buildModelConfig([entry({
      guardrails: [{ name: "pii", version: "v1", settings: { mode: "block" } }],
    })], "Development");
    expect(out?.[0].configuration?.policies).toEqual([
      { name: "pii", version: "v1", paths: [{ path: "/*", methods: ["*"], params: { mode: "block" } }] },
    ]);
  });

  it("returns undefined when no providers", () => {
    expect(buildModelConfig([], "Development")).toBeUndefined();
  });

  it("returns undefined when no provider is selected for the lowest environment", () => {
    expect(buildModelConfig([entry()], "Production")).toBeUndefined();
  });
});

describe("findLowestEnvironmentName", () => {
  it("returns the source environment that is not a promotion target", () => {
    expect(findLowestEnvironmentName([
      { sourceEnvironmentRef: "Development", targetEnvironmentRefs: [{ name: "Staging" }] },
      { sourceEnvironmentRef: "Staging", targetEnvironmentRefs: [{ name: "Production" }] },
    ])).toBe("Development");
  });

  it("returns undefined when no lowest environment can be resolved", () => {
    expect(findLowestEnvironmentName([])).toBeUndefined();
  });
});

function baseFormData(over: Partial<CreateAgentFormValues> = {}): CreateAgentFormValues {
  return {
    deploymentType: "new",
    enableAutoInstrumentation: true,
    name: "test-agent",
    displayName: "Test Agent",
    description: "",
    repositoryUrl: "https://github.com/wso2/agent-catalog-template",
    branch: "main",
    appPath: "/",
    runCommand: "python main.py",
    language: "python",
    languageVersion: "3.11",
    dockerfilePath: "/Dockerfile",
    interfaceType: "DEFAULT",
    basePath: "/",
    openApiPath: "",
    env: [],
    files: [],
    ...over,
  } as CreateAgentFormValues;
}

describe("buildCatalogAgentPayload", () => {
  const params: OrgProjPathParams = { orgName: "org1", projName: "proj1" };

  it("sends no extension block when no provider or proxy is selected", () => {
    const { body } = buildCatalogAgentPayload(baseFormData(), params, "my-kind", "v1");

    expect(body.extensions).toBeUndefined();
    expect(body).not.toHaveProperty("configurations");
  });

  it("puts the LLM bindings in the ai-gateway extension block", () => {
    const { body } = buildCatalogAgentPayload(
      baseFormData(),
      params,
      "my-kind",
      "v1",
      [entry()],
      "Development",
    );

    expect(body).not.toHaveProperty("modelConfig");
    expect(body.extensions?.["ai-gateway"]?.modelConfig?.map((m) => m.providerName))
      .toEqual(["openai"]);
  });
});

function schemaItem(
  over: Partial<AgentKindConfigSchemaItem> & { name: string },
): AgentKindConfigSchemaItem {
  return {
    isSecret: false,
    isMandatory: false,
    ...over,
  };
}

describe("deriveCatalogEnvSeed", () => {
  it("seeds a non-secret item's real default value", () => {
    const seed = deriveCatalogEnvSeed([
      schemaItem({ name: "MODEL_NAME", defaultValue: "gpt-4" }),
    ]);

    expect(seed.env).toEqual([
      { key: "MODEL_NAME", value: "gpt-4", isSensitive: false, isInherited: false },
    ]);
    expect(seed.lockedEnvKeys.has("MODEL_NAME")).toBe(true);
    expect(seed.kindSecretKeysWithDefault.has("MODEL_NAME")).toBe(false);
  });

  // isSecret alone must not be enough to lock the field — a mandatory secret with no
  // default would otherwise look identical to one that has a default, even though
  // there is nothing to fall back on if the user leaves it untouched. Only an item
  // that is BOTH secret AND has a real default should end up locked.
  it("does not lock a secret item that has no default — nothing to fall back on", () => {
    const seed = deriveCatalogEnvSeed([
      schemaItem({ name: "OPENAI_API_KEY", isSecret: true, isMandatory: true }),
    ]);

    expect(seed.env).toEqual([
      { key: "OPENAI_API_KEY", value: "", isSensitive: true, isInherited: false },
    ]);
    expect(seed.lockedEnvKeys.has("OPENAI_API_KEY")).toBe(true);
    expect(seed.kindSecretKeysWithDefault.has("OPENAI_API_KEY")).toBe(false);
  });

  it("locks a secret item that has a real default, without seeding its value", () => {
    const seed = deriveCatalogEnvSeed([
      schemaItem({ name: "OPENAI_API_KEY", isSecret: true, defaultValue: "••••••••" }),
    ]);

    expect(seed.env).toEqual([
      { key: "OPENAI_API_KEY", value: "", isSensitive: true, isInherited: true },
    ]);
    expect(seed.kindSecretKeysWithDefault.has("OPENAI_API_KEY")).toBe(true);
  });

  // Only a secret with a default inherits; a non-secret default is seeded as a real value.
  it("marks only the secret-with-default row as inherited in a mixed schema", () => {
    const seed = deriveCatalogEnvSeed([
      schemaItem({ name: "MODEL_NAME", defaultValue: "gpt-4" }),
      schemaItem({ name: "OPENAI_API_KEY", isSecret: true, defaultValue: "••••••••" }),
      schemaItem({ name: "TAVILY_API_KEY", isSecret: true, isMandatory: true }),
    ]);

    expect(seed.env).toEqual([
      { key: "MODEL_NAME", value: "gpt-4", isSensitive: false, isInherited: false },
      { key: "OPENAI_API_KEY", value: "", isSensitive: true, isInherited: true },
      { key: "TAVILY_API_KEY", value: "", isSensitive: true, isInherited: false },
    ]);
    expect([...seed.kindSecretKeysWithDefault]).toEqual(["OPENAI_API_KEY"]);
  });

  it("returns an empty seed for an empty schema", () => {
    const seed = deriveCatalogEnvSeed([]);

    expect(seed.env).toEqual([]);
    expect(seed.lockedEnvKeys.size).toBe(0);
    expect(seed.kindSecretKeysWithDefault.size).toBe(0);
  });
});

function mcpEntry(over: Partial<MCPProxyFormEntry> = {}): MCPProxyFormEntry {
  return {
    selectedProxyByEnv: { Development: { id: "proxy-1", name: "weather" } },
    urlVarName: "AGENT_MCP_1_URL",
    apikeyVarName: "AGENT_MCP_1_API_KEY",
    authenticationType: "apiKey",
    ...over,
  };
}

// Issue #1597: only an API-key endpoint has a key the user names. For OAuth and
// for "None", submitting one makes ensureMCPEnvVarRows persist it with an empty
// secret reference, which mcpProxyAPIKeySecurityEnabled then never fills — the
// agent gets a variable that is empty forever, with no error at create or deploy.
describe("buildMCPConfig", () => {
  it("submits both url and apikey for an API-key-secured proxy", () => {
    expect(buildMCPConfig([mcpEntry()], "Development")).toEqual([
      {
        proxyName: "proxy-1",
        environmentVariables: [
          { key: "url", name: "AGENT_MCP_1_URL" },
          { key: "apikey", name: "AGENT_MCP_1_API_KEY" },
        ],
      },
    ]);
  });

  it("omits apikey for an OAuth proxy even when a stale name is still in form state", () => {
    expect(
      buildMCPConfig([mcpEntry({ authenticationType: "identity" })], "Development"),
    ).toEqual([
      {
        proxyName: "proxy-1",
        environmentVariables: [{ key: "url", name: "AGENT_MCP_1_URL" }],
      },
    ]);
  });

  it("omits apikey for an unsecured proxy even when a stale name is still in form state", () => {
    expect(
      buildMCPConfig([mcpEntry({ authenticationType: "" })], "Development"),
    ).toEqual([
      {
        proxyName: "proxy-1",
        environmentVariables: [{ key: "url", name: "AGENT_MCP_1_URL" }],
      },
    ]);
  });

  // A submit that races the proxy fetch must fail safe: no resolved security
  // means no API key variable, rather than one the platform injects empty.
  it("omits apikey when the proxy's security has not resolved yet", () => {
    expect(
      buildMCPConfig([mcpEntry({ authenticationType: undefined })], "Development"),
    ).toEqual([
      {
        proxyName: "proxy-1",
        environmentVariables: [{ key: "url", name: "AGENT_MCP_1_URL" }],
      },
    ]);
  });

  it("still sends the url for every security kind, since the agent always needs it", () => {
    for (const authenticationType of ["apiKey", "identity", ""] as const) {
      const out = buildMCPConfig([mcpEntry({ authenticationType })], "Development");
      expect(out?.[0]?.environmentVariables).toContainEqual({
        key: "url",
        name: "AGENT_MCP_1_URL",
      });
    }
  });

  it("builds nothing when the initial environment has no proxy selected", () => {
    expect(buildMCPConfig([mcpEntry()], "Production")).toBeUndefined();
  });
});

describe("mcpEntryVarNames", () => {
  it("reserves both names for an API-key entry", () => {
    expect(mcpEntryVarNames(mcpEntry(), 0, "AGENT")).toEqual([
      "AGENT_MCP_1_URL",
      "AGENT_MCP_1_API_KEY",
    ]);
  });

  it("reserves only the url for an OAuth entry, freeing the api key name", () => {
    expect(
      mcpEntryVarNames(mcpEntry({ authenticationType: "identity" }), 0, "AGENT"),
    ).toEqual(["AGENT_MCP_1_URL"]);
  });

  it("reserves only the url for an unsecured entry", () => {
    expect(
      mcpEntryVarNames(mcpEntry({ authenticationType: "" }), 0, "AGENT"),
    ).toEqual(["AGENT_MCP_1_URL"]);
  });

  it("falls back to index-derived defaults when names are unset", () => {
    expect(
      mcpEntryVarNames(
        mcpEntry({ urlVarName: undefined, apikeyVarName: undefined }),
        1,
        "MYAGENT",
      ),
    ).toEqual(["MYAGENT_MCP_2_URL", "MYAGENT_MCP_2_API_KEY"]);
  });
});

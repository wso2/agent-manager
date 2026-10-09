import { describe, expect, it, vi } from "vitest";

// The shared-component barrel pulls in UI dependencies that do not load under
// vitest here; only the name mapping (tested in shared-component) is needed.
vi.mock("@agent-management-platform/shared-component", () => ({
  ballerinaConfigVarName: (name: string) => `BAL_CONFIG_VAR_${name.toUpperCase()}`,
}));

import { envVarNaming, toCamelCase } from "./envVarNaming";

describe("toCamelCase", () => {
  it("camel-cases a display name", () => {
    expect(toCamelCase("My agent-1")).toBe("myAgent1");
    expect(toCamelCase("Leave Assistant")).toBe("leaveAssistant");
    expect(toCamelCase("GPT helper")).toBe("gptHelper");
    expect(toCamelCase("")).toBe("");
  });
});

describe("envVarNaming", () => {
  it("derives a Ballerina agent's names from camelCase configurables", () => {
    const naming = envVarNaming("Leave Assistant", "ballerina");
    expect(naming.configurable("llmUrl", 0)).toBe("leaveAssistant1Url");
    expect(naming.name("llmUrl", 0)).toBe("BAL_CONFIG_VAR_LEAVEASSISTANT1URL");
    expect(naming.configurable("llmApiKey", 1)).toBe("leaveAssistant2ApiKey");
    expect(naming.configurable("mcpUrl", 0)).toBe("leaveAssistantMcp1Url");
    expect(naming.name("mcpApiKey", 0)).toBe("BAL_CONFIG_VAR_LEAVEASSISTANTMCP1APIKEY");
  });

  it("falls back to an agent base for a Ballerina agent without a usable name", () => {
    expect(envVarNaming("", "ballerina").configurable("llmUrl", 0)).toBe("agent1Url");
    expect(envVarNaming("123", "ballerina").configurable("llmUrl", 0)).toBe("agent1231Url");
  });

  it("keeps the upper-case names for other languages", () => {
    const naming = envVarNaming("My Agent", "python");
    expect(naming.name("llmUrl", 0)).toBe("MY_AGENT_1_URL");
    expect(naming.name("mcpApiKey", 1)).toBe("MY_AGENT_MCP_2_API_KEY");
    expect(naming.configurable("llmUrl", 0)).toBeUndefined();
    expect(envVarNaming(undefined, undefined).name("llmApiKey", 0)).toBe("AGENT_1_API_KEY");
  });
});

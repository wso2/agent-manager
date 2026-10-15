import { describe, expect, it, vi } from "vitest";

// The shared-component barrel pulls in UI dependencies that do not load under
// vitest here; only the name mapping (tested in shared-component) is needed.
vi.mock("@agent-management-platform/shared-component", () => ({
  ballerinaConfigVarName: (name: string) => `BAL_CONFIG_VAR_${name.toUpperCase()}`,
}));

import { envVarNaming, renameGeneratedVarNames, toCamelCase } from "./envVarNaming";

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

  it("keeps every generated name within the 64-character variable-name limit", () => {
    const longName = "A very long agent display name ".repeat(4).slice(0, 100);
    const kinds = ["llmUrl", "llmApiKey", "mcpUrl", "mcpApiKey"] as const;
    for (const language of ["ballerina", "python"]) {
      const naming = envVarNaming(longName, language);
      for (const kind of kinds) {
        for (const index of [0, 98]) {
          expect(naming.name(kind, index).length).toBeLessThanOrEqual(64);
        }
      }
    }
  });
});

describe("renameGeneratedVarNames", () => {
  const python = envVarNaming("Leave Assistant", "python");
  const ballerina = envVarNaming("Leave Assistant", "ballerina");
  const llmKinds = { url: "llmUrl", apikey: "llmApiKey" } as const;

  it("moves names still at their defaults to the new language's defaults", () => {
    const entries = [
      { urlVarName: "LEAVE_ASSISTANT_1_URL", apikeyVarName: "LEAVE_ASSISTANT_1_API_KEY" },
      { urlVarName: "LEAVE_ASSISTANT_2_URL", apikeyVarName: "LEAVE_ASSISTANT_2_API_KEY" },
    ];
    expect(renameGeneratedVarNames(entries, python, ballerina, llmKinds)).toEqual([
      { urlVarName: "BAL_CONFIG_VAR_LEAVEASSISTANT1URL", apikeyVarName: "BAL_CONFIG_VAR_LEAVEASSISTANT1APIKEY" },
      { urlVarName: "BAL_CONFIG_VAR_LEAVEASSISTANT2URL", apikeyVarName: "BAL_CONFIG_VAR_LEAVEASSISTANT2APIKEY" },
    ]);
  });

  it("still recognises defaults that shifted when an earlier entry was removed", () => {
    // The first entry was removed, so the second (with entry-2 names) is now first.
    const entries = [
      { urlVarName: "LEAVE_ASSISTANT_2_URL", apikeyVarName: "LEAVE_ASSISTANT_2_API_KEY" },
    ];
    expect(renameGeneratedVarNames(entries, python, ballerina, llmKinds)).toEqual([
      { urlVarName: "BAL_CONFIG_VAR_LEAVEASSISTANT2URL", apikeyVarName: "BAL_CONFIG_VAR_LEAVEASSISTANT2APIKEY" },
    ]);
  });

  it("keeps names the user edited", () => {
    const entries = [{ urlVarName: "MY_URL", apikeyVarName: "LEAVE_ASSISTANT_1_API_KEY" }];
    expect(renameGeneratedVarNames(entries, python, ballerina, llmKinds)).toEqual([
      { urlVarName: "MY_URL", apikeyVarName: "BAL_CONFIG_VAR_LEAVEASSISTANT1APIKEY" },
    ]);
  });

  it("leaves a name that is not set yet unset", () => {
    const entries = [{ urlVarName: "LEAVE_ASSISTANT_MCP_1_URL" }];
    const renamed = renameGeneratedVarNames(entries, python, ballerina, { url: "mcpUrl", apikey: "mcpApiKey" });
    expect(renamed).toStrictEqual([{ urlVarName: "BAL_CONFIG_VAR_LEAVEASSISTANTMCP1URL" }]);
  });

  it("returns the same array when nothing changed", () => {
    const entries = [{ urlVarName: "MY_URL", apikeyVarName: "MY_KEY" }];
    expect(renameGeneratedVarNames(entries, python, ballerina, llmKinds)).toBe(entries);
  });
});

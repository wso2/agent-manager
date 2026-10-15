import { describe, it, expect } from "vitest";
import { buildAgentCreationPayload } from "./buildAgentPayload";
import type { AddAgentFormValues } from "../form/schema";
import type { OrgProjPathParams } from "@agent-management-platform/types";

const params = { orgName: "acme", projName: "checkout" } as OrgProjPathParams;

function form(over: Partial<AddAgentFormValues> = {}): AddAgentFormValues {
  return {
    deploymentType: "new",
    name: "leave-assistant",
    displayName: "Leave Assistant",
    repositoryUrl: "https://github.com/example/bal-agent",
    branch: "main",
    appPath: "/",
    language: "ballerina",
    interfaceType: "DEFAULT",
    env: [],
    enableAutoInstrumentation: true,
    ...over,
  } as unknown as AddAgentFormValues;
}

// A BAL_CONFIG_VAR_* var with no matching configurable stops a Ballerina
// program from starting, so the opt-in is sent only when explicitly checked,
// and never for another language.
describe("buildAgentCreationPayload agentIdAsBallerinaConfigurables", () => {
  it("is sent for a Ballerina agent that opted in", () => {
    const { body } = buildAgentCreationPayload(
      form({ agentIdAsBallerinaConfigurables: true }),
      params,
    );
    expect(body.configurations?.agentIdAsBallerinaConfigurables).toBe(true);
  });

  it("is omitted when not checked", () => {
    const { body } = buildAgentCreationPayload(form(), params);
    expect(body.configurations).not.toHaveProperty("agentIdAsBallerinaConfigurables");
  });

  it("is omitted for another language even if left checked", () => {
    const { body } = buildAgentCreationPayload(
      form({
        language: "python",
        languageVersion: "3.11",
        runCommand: "python main.py",
        agentIdAsBallerinaConfigurables: true,
      }),
      params,
    );
    expect(body.configurations).not.toHaveProperty("agentIdAsBallerinaConfigurables");
  });
});

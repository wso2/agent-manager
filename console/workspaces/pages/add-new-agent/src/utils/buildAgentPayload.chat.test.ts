import { describe, it, expect } from "vitest";
import { buildAgentCreationPayload } from "./buildAgentPayload";
import type { AddAgentFormValues } from "../form/schema";
import type { OrgProjPathParams } from "@agent-management-platform/types";

const params = { orgName: "acme", projName: "school" } as OrgProjPathParams;

function chatForm(over: Partial<AddAgentFormValues> = {}): AddAgentFormValues {
  return {
    deploymentType: "new",
    name: "math-tutor",
    displayName: "Math Tutor",
    repositoryUrl: "https://github.com/example/math-tutor",
    branch: "main",
    appPath: "/",
    language: "ballerina",
    interfaceType: "DEFAULT",
    env: [],
    enableAutoInstrumentation: true,
    ...over,
  } as unknown as AddAgentFormValues;
}

// A Ballerina chat agent is a ballerina/ai ai:Listener serving POST <basePath>/chat
// on its own port; a Python chat agent uses the platform's fixed chat interface.
describe("buildAgentCreationPayload chat agent input interface", () => {
  it("sends a Ballerina chat agent's port and base path", () => {
    const { body } = buildAgentCreationPayload(
      chatForm({ port: 9191, basePath: "/math-tutor" }),
      params,
    );
    expect(body.agentType).toEqual({ type: "agent-api", subType: "chat-api" });
    expect(body.inputInterface).toEqual({ type: "HTTP", port: 9191, basePath: "/math-tutor" });
  });

  it("defaults a Ballerina chat agent to port 9090 and base path /", () => {
    const { body } = buildAgentCreationPayload(chatForm(), params);
    expect(body.inputInterface).toEqual({ type: "HTTP", port: 9090, basePath: "/" });
  });

  it("keeps a Python chat agent on the fixed chat interface", () => {
    const { body } = buildAgentCreationPayload(
      chatForm({
        language: "python",
        languageVersion: "3.11",
        runCommand: "python main.py",
        port: 9191,
        basePath: "/math-tutor",
      }),
      params,
    );
    expect(body.inputInterface).toEqual({ type: "HTTP" });
  });
});

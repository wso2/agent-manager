import { describe, it, expect } from "vitest";
import { buildAgentCreationPayload } from "./buildAgentPayload";
import type { AddAgentFormValues } from "../form/schema";
import type { OrgProjPathParams } from "@agent-management-platform/types";

const params = { orgName: "acme", projName: "checkout" } as OrgProjPathParams;

function a2aForm(over: Partial<AddAgentFormValues> = {}): AddAgentFormValues {
  return {
    deploymentType: "new",
    name: "trip-planner",
    displayName: "Trip Planner",
    repositoryUrl: "https://github.com/example/a2a-agent",
    branch: "main",
    appPath: "/",
    language: "python",
    languageVersion: "3.11",
    runCommand: "python main.py",
    interfaceType: "A2A",
    port: 9099,
    env: [],
    enableAutoInstrumentation: true,
    ...over,
  } as unknown as AddAgentFormValues;
}

describe("buildAgentCreationPayload for an A2A agent", () => {
  it("sends the a2a-agent subtype", () => {
    const { body } = buildAgentCreationPayload(a2aForm(), params);
    expect(body.agentType).toEqual({ type: "agent-api", subType: "a2a-agent" });
  });

  // An A2A agent serves its own agent card, so there is no OpenAPI path to send.
  it("sends a port and no schema", () => {
    const { body } = buildAgentCreationPayload(a2aForm(), params);
    expect(body.inputInterface).toEqual({ type: "HTTP", port: 9099 });
  });

  // The two-way ternary this replaces mapped anything that was not CUSTOM to
  // chat-api, so the other two interface types are pinned alongside.
  it("leaves the chat and custom interfaces as they were", () => {
    const chat = buildAgentCreationPayload(a2aForm({ interfaceType: "DEFAULT" }), params);
    expect(chat.body.agentType).toEqual({ type: "agent-api", subType: "chat-api" });
    expect(chat.body.inputInterface).toEqual({ type: "HTTP" });

    const custom = buildAgentCreationPayload(
      a2aForm({ interfaceType: "CUSTOM", basePath: "/api", openApiPath: "/openapi.yaml" }),
      params,
    );
    expect(custom.body.agentType).toEqual({ type: "agent-api", subType: "custom-api" });
    expect(custom.body.inputInterface).toEqual({
      type: "HTTP",
      port: 9099,
      basePath: "/api",
      schema: { path: "/openapi.yaml" },
    });
  });
});

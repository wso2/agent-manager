import { describe, it, expect } from "vitest";
import { buildAgentCreationPayload } from "./buildAgentPayload";
import type { AddAgentFormValues } from "../form/schema";
import type { OrgProjPathParams } from "@agent-management-platform/types";

const params = { orgName: "acme", projName: "checkout" } as OrgProjPathParams;

function chatAgentForm(over: Partial<AddAgentFormValues> = {}): AddAgentFormValues {
  return {
    deploymentType: "new",
    name: "support-bot",
    displayName: "Support Bot",
    repositoryUrl: "https://github.com/example/chat-agent",
    branch: "main",
    appPath: "/",
    language: "python",
    languageVersion: "3.11",
    runCommand: "python main.py",
    interfaceType: "DEFAULT",
    env: [],
    enableAutoInstrumentation: true,
    ...over,
  } as unknown as AddAgentFormValues;
}

describe("buildAgentCreationPayload health checks", () => {
  // An agent created without touching the readiness check is saved exactly as before.
  it("sends no health checks when readiness is left at the default", () => {
    expect(buildAgentCreationPayload(chatAgentForm(), params).body).not.toHaveProperty("healthChecks");
    expect(
      buildAgentCreationPayload(chatAgentForm({ healthChecks: { readiness: {} } }), params).body,
    ).not.toHaveProperty("healthChecks");
  });

  it("sends the readiness check when it was changed", () => {
    const { body } = buildAgentCreationPayload(
      chatAgentForm({ healthChecks: { readiness: { type: "HTTP", path: "/health", port: 8000 } } }),
      params,
    );
    expect(body.healthChecks).toEqual({ readiness: { type: "HTTP", path: "/health", port: 8000 } });
  });
});

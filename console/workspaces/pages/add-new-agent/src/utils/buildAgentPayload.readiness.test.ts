import { describe, it, expect } from "vitest";
import { buildAgentCreationPayload } from "./buildAgentPayload";
import { READINESS_BY_SUBTYPE, readinessOptionFor } from "./readinessCheck";
import type { AddAgentFormValues } from "../form/schema";
import type { OrgProjPathParams } from "@agent-management-platform/types";

const params = { orgName: "acme", projName: "checkout" } as OrgProjPathParams;

function form(over: Partial<AddAgentFormValues> = {}): AddAgentFormValues {
  return {
    deploymentType: "new",
    name: "trip-planner",
    displayName: "Trip Planner",
    repositoryUrl: "https://github.com/example/agent",
    branch: "main",
    appPath: "/",
    language: "python",
    languageVersion: "3.11",
    runCommand: "python main.py",
    interfaceType: "DEFAULT",
    readinessType: "tcp",
    readinessPath: "/health",
    env: [],
    enableAutoInstrumentation: true,
    ...over,
  } as unknown as AddAgentFormValues;
}

describe("readiness check on create", () => {
  it("is offered for chat, custom API and A2A agents", () => {
    expect(Object.keys(READINESS_BY_SUBTYPE).sort()).toEqual(["a2a-agent", "chat-api", "custom-api"]);
    expect(readinessOptionFor("DEFAULT")?.httpPath).toBe("/health");
    expect(readinessOptionFor("A2A")?.httpPath).toBe("/.well-known/agent-card.json");
  });

  it("sends no health checks for TCP, the platform default", () => {
    const { body } = buildAgentCreationPayload(form(), params);
    expect(body).not.toHaveProperty("healthChecks");
  });

  it("sends only the readiness type and a trimmed path for HTTP GET", () => {
    const { body } = buildAgentCreationPayload(
      form({ readinessType: "http", readinessPath: " /ready " } as Partial<AddAgentFormValues>),
      params,
    );
    expect(body.healthChecks).toEqual({ readiness: { type: "http", path: "/ready" } });
  });

  it("sends the agent card path for an A2A agent", () => {
    const { body } = buildAgentCreationPayload(
      form({
        interfaceType: "A2A",
        port: 9099,
        readinessType: "http",
        readinessPath: "/.well-known/agent-card.json",
      } as Partial<AddAgentFormValues>),
      params,
    );
    expect(body.healthChecks).toEqual({
      readiness: { type: "http", path: "/.well-known/agent-card.json" },
    });
  });
});

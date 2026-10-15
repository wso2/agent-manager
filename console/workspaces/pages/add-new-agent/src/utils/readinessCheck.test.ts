import { describe, it, expect } from "vitest";
import { createAgentSchema } from "../form/schema";
import { readinessOnTypeChange } from "./readinessCheck";

describe("readinessOnTypeChange", () => {
  it("moves an untouched path to the new type's default", () => {
    expect(
      readinessOnTypeChange(
        { interfaceType: "DEFAULT", readinessType: "http", readinessPath: "/health" },
        "A2A",
      ),
    ).toEqual({ readinessType: "http", readinessPath: "/.well-known/agent-card.json" });
  });

  it("keeps a path the user typed", () => {
    expect(
      readinessOnTypeChange(
        { interfaceType: "A2A", readinessType: "http", readinessPath: "/ready" },
        "CUSTOM",
      ),
    ).toEqual({ readinessType: "http", readinessPath: "/ready" });
  });

  it("fills an emptied path with the new type's default", () => {
    expect(
      readinessOnTypeChange({ interfaceType: "CUSTOM", readinessType: "tcp", readinessPath: "" }, "A2A"),
    ).toEqual({ readinessType: "tcp", readinessPath: "/.well-known/agent-card.json" });
  });
});

describe("createAgentSchema readiness check", () => {
  const valid = {
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
    env: [],
    files: [],
  };
  const readinessPathErrors = (over: Record<string, unknown>) => {
    const result = createAgentSchema.safeParse({ ...valid, ...over });
    return result.success
      ? []
      : result.error.issues
        .filter((issue) => issue.path.join(".") === "readinessPath")
        .map((issue) => issue.message);
  };

  it("accepts TCP with no path", () => {
    expect(readinessPathErrors({ readinessType: "tcp", readinessPath: "" })).toEqual([]);
  });

  it("accepts an HTTP GET path", () => {
    expect(
      readinessPathErrors({ readinessType: "http", readinessPath: "/.well-known/agent-card.json" }),
    ).toEqual([]);
  });

  it("requires a path for HTTP GET", () => {
    expect(readinessPathErrors({ readinessType: "http", readinessPath: "  " })).toEqual([
      "Path is required for an HTTP GET check",
    ]);
  });

  it("limits the path to 1024 characters, as the backend does", () => {
    expect(
      readinessPathErrors({ readinessType: "http", readinessPath: "/" + "a".repeat(1024) }),
    ).toEqual(["Path must be at most 1024 characters"]);
  });

  it("requires the path to start with /", () => {
    expect(readinessPathErrors({ readinessType: "http", readinessPath: "health" })).toEqual([
      "Path must start with / and contain no spaces",
    ]);
  });
});

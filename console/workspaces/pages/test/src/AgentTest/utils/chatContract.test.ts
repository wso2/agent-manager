import { describe, expect, it } from "vitest";
import {
  buildChatRequestBody,
  chatContractFor,
  readChatReply,
} from "./chatContract";

describe("chatContractFor", () => {
  it("uses the ai:Listener contract for a Ballerina buildpack agent", () => {
    expect(chatContractFor({ type: "buildpack", buildpack: { language: "ballerina" } })).toBe(
      "ballerina",
    );
  });

  it("uses the standard contract for everything else", () => {
    expect(chatContractFor({ type: "buildpack", buildpack: { language: "python" } })).toBe(
      "standard",
    );
    expect(chatContractFor({ type: "docker", docker: { dockerfilePath: "/Dockerfile" } })).toBe(
      "standard",
    );
    expect(chatContractFor(undefined)).toBe("standard");
  });
});

describe("buildChatRequestBody", () => {
  // ai:Listener rejects the standard contract's session_id and context fields.
  it("sends only message and sessionId to a Ballerina agent", () => {
    expect(buildChatRequestBody("ballerina", "session-7", "hi")).toEqual({
      message: "hi",
      sessionId: "session-7",
    });
  });

  it("sends session_id, message and context to a standard agent", () => {
    expect(buildChatRequestBody("standard", "session-7", "hi")).toEqual({
      session_id: "session-7",
      message: "hi",
      context: {},
    });
  });
});

describe("readChatReply", () => {
  it("reads message from a Ballerina reply and response from a standard one", () => {
    expect(readChatReply("ballerina", { message: "4" })).toBe("4");
    expect(readChatReply("standard", { response: "4" })).toBe("4");
  });

  it("is undefined when the reply has the other contract's shape", () => {
    expect(readChatReply("ballerina", { response: "4" })).toBeUndefined();
    expect(readChatReply("standard", { message: "4" })).toBeUndefined();
    expect(readChatReply("standard", "plain text")).toBeUndefined();
  });
});

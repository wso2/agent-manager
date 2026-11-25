/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { describe, expect, it } from "vitest";
import { insertLink, prefixLines, wrapSelection } from "./textEditActions";

describe("wrapSelection", () => {
  it("wraps the selected text and keeps it selected", () => {
    expect(wrapSelection({ text: "make this bold", start: 5, }, 9, "**", "**", "bold text")).toEqual({
      text: "make **this** bold",
      selectionStart: 7,
      selectionEnd: 11,
    });
  });

  it("inserts and selects the placeholder for an empty selection", () => {
    expect(wrapSelection({ text: "ab", start: 1 }, 1, "*", "*", "italic text")).toEqual({
      text: "a*italic text*b",
      selectionStart: 2,
      selectionEnd: 13,
    });
  });
});

describe("prefixLines", () => {
  it.each([
    ["the caret's line", "one\ntwo\nthree", 5, 5, "one\n- two\nthree", 7, 7],
    ["every selected line", "one\ntwo\nthree", 1, 6, "- one\n- two\nthree", 3, 10],
    ["the last line", "one\ntwo", 6, 7, "one\n- two", 8, 9],
    ["an empty document", "", 0, 0, "- ", 2, 2],
  ])("prefixes %s", (_label, text, start, end, expected, selStart, selEnd) => {
    expect(prefixLines({ text, start }, end, "- ")).toEqual({
      text: expected,
      selectionStart: selStart,
      selectionEnd: selEnd,
    });
  });
});

describe("insertLink", () => {
  it("uses the selection as link text and selects the url", () => {
    const result = insertLink({ text: "see docs here", start: 4 }, 8);
    expect(result.text).toBe("see [docs](url) here");
    expect(result.text.slice(result.selectionStart, result.selectionEnd)).toBe("url");
  });

  it("falls back to placeholder link text", () => {
    const result = insertLink({ text: "", start: 0 }, 0);
    expect(result.text).toBe("[link text](url)");
    expect(result.text.slice(result.selectionStart, result.selectionEnd)).toBe("url");
  });
});

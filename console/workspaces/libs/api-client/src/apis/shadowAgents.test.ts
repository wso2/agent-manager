/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { httpGET, httpPOSTForm } from "../utils";
import {
  contentDispositionFileName,
  fetchShadowInventoryTemplate,
  uploadShadowInventory,
} from "./shadowAgents";

vi.mock("../utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../utils")>()),
  httpGET: vi.fn(),
  httpPOSTForm: vi.fn(),
}));

const BASE = "/api/v1/orgs/acme/shadow-agents";
const getToken = async () => "jwt";

beforeEach(() => {
  vi.mocked(httpGET).mockReset();
  vi.mocked(httpPOSTForm).mockReset();
});

describe("Bring your own inventory API", () => {
  it("fetches the template in the format asked, named as the server says", async () => {
    vi.mocked(httpGET).mockResolvedValue(
      new Response("id,name\n", {
        headers: { "Content-Disposition": 'attachment; filename="agent-inventory-template.csv"' },
      }),
    );
    const { blob, fileName } = await fetchShadowInventoryTemplate("acme", "csv", getToken);
    expect(httpGET).toHaveBeenCalledWith(`${BASE}/inventory/template`, {
      searchParams: { format: "csv" },
      token: "jwt",
    });
    expect(fileName).toBe("agent-inventory-template.csv");
    // jsdom's Blob has no text(): its size is the 8 bytes sent.
    expect(blob.size).toBe(8);
  });

  it("names the template by its format when the server gives no name", async () => {
    vi.mocked(httpGET).mockResolvedValue(new Response("x"));
    const { fileName } = await fetchShadowInventoryTemplate("acme", "xlsx", getToken);
    expect(fileName).toBe("agent-inventory-template.xlsx");
  });

  it("uploads the file as the multipart part named file", async () => {
    vi.mocked(httpPOSTForm).mockResolvedValue(new Response(JSON.stringify({ agentsSaved: 1 })));
    const file = new File(["{}"], "agents.json", { type: "application/json" });
    const r = await uploadShadowInventory("acme", "c 1", file, getToken);
    expect(r).toEqual({ agentsSaved: 1 });
    const [path, form, params] = vi.mocked(httpPOSTForm).mock.calls[0];
    expect(path).toBe(`${BASE}/connections/c%201/inventory`);
    expect(params).toEqual({ token: "jwt" });
    const part = form.get("file") as File;
    expect(part.name).toBe("agents.json");
    expect(part.size).toBe(2);
  });

  it("reads the file name of a Content-Disposition header", () => {
    expect(contentDispositionFileName(null)).toBeUndefined();
    expect(contentDispositionFileName("attachment")).toBeUndefined();
    expect(contentDispositionFileName("attachment; filename=a.json")).toBe("a.json");
    expect(contentDispositionFileName("attachment; filename=\"a b.csv\"")).toBe("a b.csv");
    expect(
      contentDispositionFileName("attachment; filename=x; filename*=UTF-8''t%C3%A9.xlsx"),
    ).toBe("té.xlsx");
  });
});

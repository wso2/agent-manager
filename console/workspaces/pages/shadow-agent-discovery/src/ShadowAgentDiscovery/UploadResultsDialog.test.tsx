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
import { act, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import type { ShadowScanResponse } from "@agent-management-platform/types";

const SCAN_ID = "0b6c2f3e-1a2b-4c5d-8e9f-0a1b2c3d4e5f";
const OTHER_ID = "9f8e7d6c-5b4a-4321-8fed-cba987654321";

const scan = (over: Partial<ShadowScanResponse> = {}): ShadowScanResponse => ({
  id: SCAN_ID,
  status: "pending",
  connectionIds: ["c1"],
  results: [],
  findingsCount: 0,
  requestedBy: "",
  createdAt: "2026-11-20T00:00:00Z",
  ...over,
});

const results = {
  complete: true,
  scannedRepositories: ["api", "web"],
  failedRepositories: [{ name: "old", reason: "archive too large" }],
  agents: [],
};

const resultsFile = (over: Record<string, unknown> = {}) =>
  JSON.stringify({
    format: "amp-shadow-scan-results",
    version: 1,
    scanId: SCAN_ID,
    createdAt: "2026-11-20T01:00:00Z",
    summary: { organization: "acme", repositoriesScanned: 12, repositoriesFailed: 1, agents: 7 },
    results,
    ...over,
  });

const hoisted = vi.hoisted(() => ({
  scans: [] as ShadowScanResponse[],
  lookup: { data: undefined, isLoading: false, error: null } as {
    data: ShadowScanResponse | undefined;
    isLoading: boolean;
    error: unknown;
  },
  lookupIds: [] as (string | undefined)[],
  upload: vi.fn(),
  resetUpload: vi.fn(),
  cancel: vi.fn(),
  clear: vi.fn(),
  addConfirmation: vi.fn(),
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useShadowScans: () => ({ data: { scans: hoisted.scans }, isLoading: false, error: null }),
  useShadowScan: (_org: string | undefined, id: string | undefined) => {
    hoisted.lookupIds.push(id);
    return id ? hoisted.lookup : { data: undefined, isLoading: false, error: null };
  },
  useClearShadowScans: () => ({ mutate: hoisted.clear, isPending: false }),
  useCancelShadowScan: () => ({ mutate: hoisted.cancel, isPending: false }),
  useUploadShadowScanResults: () => ({
    mutate: hoisted.upload,
    reset: hoisted.resetUpload,
    isPending: false,
  }),
  isShadowScanActive: (s: string | undefined) => s === "queued" || s === "running",
  isRequestTooLargeError: (e: unknown) =>
    (e as { code?: string } | null)?.code === "REQUEST_TOO_LARGE",
}));

vi.mock("@agent-management-platform/shared-component", async () => {
  const React = await import("react");
  return {
    ResourceListShell: ({
      toolbar,
      children,
    }: {
      toolbar?: React.ReactNode;
      children: React.ReactNode;
    }) => React.createElement("div", null, toolbar, children),
    isListEmpty: () => false,
    formatRelativeTime: () => "2 days ago",
    useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
  };
});

import {
  MAX_RESULTS_BYTES,
  MAX_RESULTS_FILE_BYTES,
  parseResultsFile,
  uploadErrorMessage,
  UploadResultsDialog,
} from "./subComponents/UploadResultsDialog";
import { ScansTab } from "./subComponents/ScansTab";

type UploadOptions = { onSuccess: () => void; onError: (e: unknown) => void };

const fileInput = () => screen.getByTestId("results-file-input") as HTMLInputElement;

async function pick(text: string, size?: number) {
  const f = new File([text], `results-${SCAN_ID}.json`, { type: "application/json" });
  if (size !== undefined) Object.defineProperty(f, "size", { value: size });
  await act(async () => {
    fireEvent.change(fileInput(), { target: { files: [f] } });
    // FileReader answers on a later task.
    await new Promise((r) => setTimeout(r, 20));
  });
}

function openDialog(props: { scan?: ShadowScanResponse | null; onClose?: () => void } = {}) {
  return render(
    <UploadResultsDialog
      orgName="acme"
      open
      scan={props.scan}
      scans={hoisted.scans}
      onClose={props.onClose ?? vi.fn()}
    />,
  );
}

const uploadButton = () => screen.getByRole("button", { name: "Upload" });

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.scans = [scan()];
  hoisted.lookup = { data: undefined, isLoading: false, error: null };
  hoisted.lookupIds = [];
});

describe("parseResultsFile", () => {
  it("accepts a version 1 results file", () => {
    const r = parseResultsFile(resultsFile());
    expect(r.ok && r.file.scanId).toBe(SCAN_ID);
  });

  it("names what is wrong with other files", () => {
    expect(parseResultsFile("{nope")).toMatchObject({ ok: false, error: /not valid JSON/ });
    expect(parseResultsFile(JSON.stringify({ scanId: SCAN_ID }))).toMatchObject({
      ok: false,
      error: /not a scan results file/,
    });
    expect(parseResultsFile(resultsFile({ version: 2 }))).toMatchObject({
      ok: false,
      error: /version Agent Manager cannot read/,
    });
    expect(parseResultsFile(resultsFile({ scanId: "../x" }))).toMatchObject({
      ok: false,
      error: /incomplete/,
    });
    expect(parseResultsFile(resultsFile({ results: [] }))).toMatchObject({
      ok: false,
      error: /incomplete/,
    });
  });

  it("measures the results, not the file, against 8 MiB", () => {
    // UTF-8 bytes: each "é" is two.
    const big = { ...results, padding: "é".repeat(MAX_RESULTS_BYTES / 2) };
    expect(parseResultsFile(resultsFile({ results: big }))).toMatchObject({
      ok: false,
      error: /results in this file are larger than 8 MiB/,
    });
    // Pretty-printing makes the file larger, not the results.
    expect(parseResultsFile(JSON.stringify(JSON.parse(resultsFile()), null, 8)).ok).toBe(true);
  });
});

describe("uploadErrorMessage", () => {
  const err = (status: number, message = "") => Object.assign(new Error(message), { status });

  it("explains the server's refusals", () => {
    expect(
      uploadErrorMessage(err(409, "a newer scan of this platform has already published results")),
    ).toMatch(/A newer scan of this platform has already uploaded its results/);
    expect(uploadErrorMessage(err(409, "the scan is not waiting for results"))).toBe(
      "This scan is no longer waiting for results.",
    );
    expect(uploadErrorMessage(err(413))).toBe("The results are larger than Agent Manager accepts.");
    expect(uploadErrorMessage(err(400, "invalid input: agents[0].name is empty"))).toBe(
      "Agent Manager rejected the results: invalid input: agents[0].name is empty",
    );
    expect(uploadErrorMessage(err(400, "HTTP error! status: 400"))).toMatch(
      /rejected the results\. Scan again/,
    );
    expect(uploadErrorMessage(err(500))).toBe("The results could not be uploaded. Try again.");
  });

  it("names the console's own size refusal", () => {
    const e = Object.assign(new Error("Request is too large (9 MB of 5 MB)."), {
      status: 413,
      code: "REQUEST_TOO_LARGE",
    });
    expect(uploadErrorMessage(e)).toBe(
      "The results are larger than this Agent Manager accepts from the console.",
    );
  });
});

describe("UploadResultsDialog", () => {
  it("previews the file of the scan it was opened for and uploads only its results", async () => {
    const onClose = vi.fn();
    openDialog({ scan: scan(), onClose });
    expect(uploadButton()).toBeDisabled();
    await pick(resultsFile());
    expect(screen.getByText("acme")).toBeInTheDocument();
    expect(screen.getByText("2 days ago")).toBeInTheDocument();
    expect(screen.getByText("12 scanned, 1 failed")).toBeInTheDocument();
    expect(screen.getByText("7")).toBeInTheDocument();
    fireEvent.click(uploadButton());
    expect(hoisted.upload).toHaveBeenCalledWith(
      { scanId: SCAN_ID, results },
      expect.anything(),
    );
    const opts = hoisted.upload.mock.lastCall?.[1] as UploadOptions;
    act(() => opts.onSuccess());
    expect(onClose).toHaveBeenCalledTimes(1);
    // The file's results do not stay in the mutation cache.
    expect(hoisted.resetUpload).toHaveBeenCalled();
  });

  it("refuses a file of another scan when opened for one", async () => {
    openDialog({ scan: scan() });
    await pick(resultsFile({ scanId: OTHER_ID }));
    expect(screen.getByText(/This file is for another scan/)).toBeInTheDocument();
    expect(uploadButton()).toBeDisabled();
  });

  it("says when the file is not JSON, or not a results file", async () => {
    openDialog({ scan: scan() });
    await pick("not json");
    expect(screen.getByText(/This file is not valid JSON/)).toBeInTheDocument();
    await pick(JSON.stringify({ format: "something-else" }));
    expect(screen.getByText(/This is not a scan results file/)).toBeInTheDocument();
    expect(uploadButton()).toBeDisabled();
  });

  it("refuses a file larger than 32 MiB without reading it", async () => {
    openDialog({ scan: scan() });
    await pick(resultsFile(), MAX_RESULTS_FILE_BYTES + 1);
    expect(screen.getByText(/larger than 32 MiB/)).toBeInTheDocument();
    expect(uploadButton()).toBeDisabled();
  });

  it("reads a file over 8 MiB when its results fit", async () => {
    openDialog({ scan: scan() });
    await pick(resultsFile(), MAX_RESULTS_BYTES + 1);
    expect(screen.queryByText(/larger than/)).not.toBeInTheDocument();
    expect(uploadButton()).toBeEnabled();
  });

  it("finds the scan by the file's scan ID when opened without one", async () => {
    hoisted.scans = [scan({ id: OTHER_ID, status: "succeeded" }), scan()];
    openDialog();
    await pick(resultsFile({ scanId: SCAN_ID.toUpperCase() }));
    expect(screen.getByText("12 scanned, 1 failed")).toBeInTheDocument();
    fireEvent.click(uploadButton());
    expect(hoisted.upload).toHaveBeenCalledWith({ scanId: SCAN_ID, results }, expect.anything());
  });

  it("requires the scan of the file to be waiting for results", async () => {
    hoisted.scans = [scan({ status: "failed" })];
    openDialog();
    await pick(resultsFile());
    expect(
      screen.getByText("This scan is no longer waiting for results. Its status is Failed."),
    ).toBeInTheDocument();
    expect(uploadButton()).toBeDisabled();
  });

  it("looks up a scan not in the list, and says when there is none", async () => {
    hoisted.scans = [];
    hoisted.lookup = {
      data: undefined,
      isLoading: false,
      error: Object.assign(new Error("x"), { status: 404 }),
    };
    openDialog();
    await pick(resultsFile());
    expect(hoisted.lookupIds).toContain(SCAN_ID);
    expect(
      screen.getByText("No scan of this organization has this file's scan ID."),
    ).toBeInTheDocument();
    expect(uploadButton()).toBeDisabled();
  });

  it("shows the server's refusal and stays open", async () => {
    const onClose = vi.fn();
    openDialog({ scan: scan(), onClose });
    await pick(resultsFile());
    fireEvent.click(uploadButton());
    const opts = hoisted.upload.mock.lastCall?.[1] as UploadOptions;
    act(() =>
      opts.onError(
        Object.assign(new Error("a newer scan of this platform has already published results"), {
          status: 409,
        }),
      ),
    );
    expect(screen.getByText(/A newer scan of this platform/)).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
  });
});

describe("ScansTab waiting scans", () => {
  const renderTab = (canManage = true) =>
    render(
      <MemoryRouter>
        <ScansTab orgName="acme" canManage={canManage} />
      </MemoryRouter>,
    );

  it("cancels a waiting scan after asking", () => {
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Cancel the scan started 2 days ago" }));
    expect(hoisted.cancel).not.toHaveBeenCalled();
    const confirmation = hoisted.addConfirmation.mock.lastCall?.[0] as {
      confirmButtonText: string;
      onConfirm: () => void;
    };
    expect(confirmation.confirmButtonText).toBe("Cancel scan");
    act(() => confirmation.onConfirm());
    expect(hoisted.cancel).toHaveBeenCalledWith(SCAN_ID, expect.anything());
  });

  it("uploads from a row: the file must be that scan's", async () => {
    renderTab();
    fireEvent.click(
      screen.getByRole("button", { name: "Upload results of the scan started 2 days ago" }),
    );
    expect(screen.getByRole("dialog", { name: "Upload results" })).toBeInTheDocument();
    await pick(resultsFile({ scanId: OTHER_ID }));
    expect(screen.getByText(/This file is for another scan/)).toBeInTheDocument();
  });

  it("uploads from the top without picking a scan", async () => {
    hoisted.scans = [scan({ id: OTHER_ID }), scan()];
    renderTab();
    fireEvent.click(screen.getByRole("button", { name: "Upload results" }));
    await pick(resultsFile());
    fireEvent.click(uploadButton());
    expect(hoisted.upload).toHaveBeenCalledWith({ scanId: SCAN_ID, results }, expect.anything());
  });

  it("offers no upload or cancel without manage permission, or for a finished scan", () => {
    const { unmount } = renderTab(false);
    expect(screen.queryByRole("button", { name: /Upload results|Cancel the scan/ })).toBeNull();
    unmount();
    hoisted.scans = [scan({ status: "succeeded" })];
    renderTab();
    expect(screen.queryByRole("button", { name: /Cancel the scan/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /Upload results of/ })).toBeNull();
    // The top button is there without a waiting row.
    expect(screen.getByRole("button", { name: "Upload results" })).toBeInTheDocument();
  });
});

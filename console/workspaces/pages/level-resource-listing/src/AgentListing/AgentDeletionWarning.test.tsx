/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AgentDeletionWarning } from "./AgentDeletionWarning";

const outsidePipeline = {
  code: "AGENT_IDENTITY_OUTSIDE_PIPELINE",
  message:
    "The agent was deleted, but its identity may still exist in environments outside the current deployment pipeline: staging-old, preview. Remove it in those environments through your identity provider console.",
  environments: ["preview", "staging-old"],
};

describe("AgentDeletionWarning", () => {
  it("shows the whole cleanup message without truncating", () => {
    render(<AgentDeletionWarning warnings={[outsidePipeline]} />);
    expect(screen.getByRole("alert")).toHaveTextContent(outsidePipeline.message);
  });

  it("shows one alert for each warning", () => {
    render(
      <AgentDeletionWarning
        warnings={[outsidePipeline, { ...outsidePipeline, code: "OTHER", message: "Another step." }]}
      />,
    );
    expect(screen.getAllByRole("alert")).toHaveLength(2);
  });

  it("can be dismissed", () => {
    const onClose = vi.fn();
    render(<AgentDeletionWarning warnings={[outsidePipeline]} onClose={onClose} />);
    fireEvent.click(screen.getByRole("button", { name: /close/i }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("renders nothing when the delete had no warnings", () => {
    const { rerender } = render(<AgentDeletionWarning warnings={[]} />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    rerender(<AgentDeletionWarning />);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

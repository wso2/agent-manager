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

import { render, screen } from "@testing-library/react";
import { TestWrapper } from "../../../setupTests";
import { CreatedMetadata, CreatedByMetadata, UpdatedMetadata } from "./CreatedMetadata";

describe("CreatedMetadata", () => {
  it("renders nothing when createdAt is missing", () => {
    const { container } = render(<CreatedMetadata />, { wrapper: TestWrapper });
    expect(container).toBeEmptyDOMElement();
  });

  it("renders a relative time from the ISO timestamp", () => {
    const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString();
    render(<CreatedMetadata createdAt={oneHourAgo} />, { wrapper: TestWrapper });

    expect(screen.getByText(/about 1 hour ago/)).toBeInTheDocument();
  });
});

describe("UpdatedMetadata", () => {
  it("renders nothing when updatedAt is missing", () => {
    const { container } = render(<UpdatedMetadata />, { wrapper: TestWrapper });
    expect(container).toBeEmptyDOMElement();
  });

  it("renders a relative time from the ISO timestamp", () => {
    const oneHourAgo = new Date(Date.now() - 60 * 60 * 1000).toISOString();
    render(<UpdatedMetadata updatedAt={oneHourAgo} />, { wrapper: TestWrapper });

    expect(screen.getByText(/about 1 hour ago/)).toBeInTheDocument();
  });
});

describe("CreatedByMetadata", () => {
  it("renders nothing when createdBy is missing", () => {
    const { container } = render(<CreatedByMetadata />, { wrapper: TestWrapper });
    expect(container).toBeEmptyDOMElement();
  });

  it("renders the creator's display name", () => {
    render(<CreatedByMetadata createdBy="Jane Doe" />, { wrapper: TestWrapper });
    expect(screen.getByText("Jane Doe")).toBeInTheDocument();
  });
});

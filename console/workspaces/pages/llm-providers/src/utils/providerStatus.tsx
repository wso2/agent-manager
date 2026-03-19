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

import React from "react";
import { CircularProgress } from "@wso2/oxygen-ui";
import {
  CheckCircle,
  CircleQuestionMark,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";

export type ProviderStatusColor = "success" | "warning" | "error" | "default";

export function resolveProviderStatusColor(
  status: string | undefined,
): ProviderStatusColor {
  switch (status?.toLowerCase()) {
    case "active":
    case "deployed":
      return "success";
    case "pending":
      return "warning";
    case "failed":
      return "error";
    default:
      return "default";
  }
}

export function resolveProviderStatusLabel(status: string | undefined): string {
  switch (status?.toLowerCase()) {
    case "active":
    case "deployed":
      return "Active";
    case "pending":
      return "Pending";
    case "failed":
      return "Failed";
    default:
      return status
        ? status.charAt(0).toUpperCase() + status.slice(1)
        : "Unknown";
  }
}

export function resolveProviderStatusIcon(
  status: string | undefined,
): React.ReactElement {
  switch (status?.toLowerCase()) {
    case "active":
    case "deployed":
      return <CheckCircle size={16} />;
    case "pending":
      return <CircularProgress size={16} />;
    case "failed":
      return <XCircle size={16} />;
    default:
      return <CircleQuestionMark size={16} />;
  }
}

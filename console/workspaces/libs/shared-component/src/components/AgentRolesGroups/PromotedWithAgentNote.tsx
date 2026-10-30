/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { Alert, Typography } from "@wso2/oxygen-ui";

interface PromotedWithAgentNoteProps {
  /** `caption` is a single muted line for cards; `alert` is an info banner for pages. */
  variant?: "caption" | "alert";
}

/**
 * Read-only note shown in place of the environment wise roles and groups
 * surfaces when the identity promotion UI is on.
 */
export const PromotedWithAgentNote: React.FC<PromotedWithAgentNoteProps> = ({
  variant = "caption",
}) => {
  const text = "Roles and groups are managed in the first environment of the agent's deployment pipeline and promoted with the agent.";
  if (variant === "alert") {
    return (
      <Alert severity="info" sx={{ mb: 2 }}>
        {text}
      </Alert>
    );
  }
  return (
    <Typography variant="caption" color="text.disabled" display="block">
      {text}
    </Typography>
  );
};

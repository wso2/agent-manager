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

import { Alert } from "@wso2/oxygen-ui";

export function AgentDeletionWarning({ error }: { error: unknown }) {
  if (!error || typeof error !== "object" || !("body" in error)) return null;
  const body = error.body;
  if (
    !body ||
    typeof body !== "object" ||
    !("code" in body) ||
    body.code !== "AGENT_IDENTITY_OUTSIDE_PIPELINE"
  )
    return null;
  const reason =
    "reason" in body && typeof body.reason === "string"
      ? body.reason
      : "Remove this agent's identity from environments outside the deployment pipeline in your identity provider console, then retry.";
  return (
    <Alert severity="warning" variant="outlined">
      {reason}
    </Alert>
  );
}

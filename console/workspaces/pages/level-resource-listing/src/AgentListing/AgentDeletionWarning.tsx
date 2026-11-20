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

import { Alert, Stack } from "@wso2/oxygen-ui";
import type { AgentDeleteWarning } from "@agent-management-platform/types";

/**
 * Follow-up the user must do after an agent was deleted, for example removing an identity left in
 * an environment outside the project pipeline. Each message is shown in full.
 */
export function AgentDeletionWarning({
  warnings,
  onClose,
}: {
  warnings?: AgentDeleteWarning[];
  onClose?: () => void;
}) {
  if (!warnings || warnings.length === 0) return null;
  return (
    <Stack spacing={1}>
      {warnings.map((warning) => (
        <Alert
          key={`${warning.code}:${warning.environments.join(",")}`}
          severity="warning"
          variant="outlined"
          onClose={onClose}
        >
          {warning.message}
        </Alert>
      ))}
    </Stack>
  );
}

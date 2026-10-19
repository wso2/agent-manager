/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
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

import { type ReactNode } from "react";
import { Alert, Stack, Typography } from "@wso2/oxygen-ui";
import { CodeBlock } from "../CodeBlock";
import { ballerinaConfigurableSnippet } from "../../utils/ballerinaConfigurables";

interface BallerinaConfigurablesNoticeProps {
  /** The configurables the program must declare (see ballerinaConfigVarName). */
  configurableNames: readonly string[];
  /** Overrides the default explanation above the snippet. */
  description?: ReactNode;
  /** Distinguishes this snippet's copy state among siblings on the same page. */
  fieldId?: string;
}

/**
 * Warns that a Ballerina program must declare these configurables — Ballerina
 * refuses to start on an injected BAL_CONFIG_VAR_* var with no configurable —
 * and gives the declarations to copy. Renders nothing for an empty list.
 */
export function BallerinaConfigurablesNotice({
  configurableNames,
  description,
  fieldId = "ballerina-configurables",
}: BallerinaConfigurablesNoticeProps) {
  const snippet = ballerinaConfigurableSnippet(configurableNames);
  if (!snippet) return null;
  return (
    <Alert severity="warning">
      <Stack spacing={1}>
        <Typography variant="body2">
          {description ??
            "These values are injected as Ballerina configurables. Declare them in your program, or the agent will fail to start:"}
        </Typography>
        <CodeBlock
          code={snippet}
          language="text"
          fieldId={fieldId}
          analyticsId="ballerina-configurables"
        />
      </Stack>
    </Alert>
  );
}
